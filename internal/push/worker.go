// Package push owns durable, opt-in FCM HTTP v1 dispatch. It never embeds
// Firebase server credentials in an Android client or Git repository.
package push

import (
    "bytes"
    "context"
    "crypto"
    "crypto/rand"
    "crypto/rsa"
    "crypto/sha256"
    "crypto/x509"
    "encoding/base64"
    "encoding/json"
    "encoding/pem"
    "errors"
    "fmt"
    "io"
    "log"
    "net/http"
    "net/url"
    "os"
    "strings"
    "time"

    "github.com/google/uuid"
    "github.com/jackc/pgx/v5"
    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/gigabytegrove/calden/internal/secrets"
)

type serviceAccount struct {
    ProjectID string `json:"project_id"`
    ClientEmail string `json:"client_email"`
    PrivateKey string `json:"private_key"`
    TokenURI string `json:"token_uri"`
}
type dispatcher struct {
    db *pgxpool.Pool
    key []byte
    credentials serviceAccount
    project string
    accessToken string
    accessUntil time.Time
    http *http.Client
}
type delivery struct {
    NoticeID,DeviceID,RecipientID uuid.UUID
    Token string
    Title,Message,Kind string
    EventID *uuid.UUID
    CalendarID *uuid.UUID
    OccurrenceStart *time.Time
    Attempts int
}
const maxAttempts = 8

func Start(ctx context.Context, db *pgxpool.Pool, key []byte, accountFile, projectID string) {
    if strings.TrimSpace(accountFile)=="" {return} // Local live websocket remains supported.
    info,err:=os.Stat(accountFile)
    if err!=nil || info.IsDir(){log.Printf("Calden push: credentials file unavailable");return}
    if info.Mode().Perm()&0077!=0 {
        log.Printf("Calden push: service credentials must have permission 0600");return
    }
    raw,err:=os.ReadFile(accountFile)
    if err!=nil{log.Printf("Calden push: credentials unreadable");return}
    var account serviceAccount
    if json.Unmarshal(raw,&account)!=nil || account.ClientEmail=="" ||
        !strings.Contains(account.PrivateKey,"PRIVATE KEY") {
        log.Print("Calden push: invalid service-account credentials");return
    }
    if projectID=="" {projectID=account.ProjectID}
    if projectID=="" || projectID!=account.ProjectID {
        log.Print("Calden push: FCM project ID does not match service account");return
    }
    if account.TokenURI=="" {account.TokenURI="https://oauth2.googleapis.com/token"}
    if account.TokenURI!="https://oauth2.googleapis.com/token" {
        log.Print("Calden push: unexpected OAuth token endpoint");return
    }
    worker:=&dispatcher{db:db,key:key,credentials:account,project:projectID,
        http:&http.Client{Timeout:15*time.Second}}
    go func(){
        ticker:=time.NewTicker(10*time.Second)
        defer ticker.Stop()
        worker.drain(ctx)
        for {
            select{
            case <-ctx.Done():return
            case <-ticker.C:worker.drain(ctx)
            }
        }
    }()
}

func (d *dispatcher) drain(ctx context.Context) {
    for i:=0;i<50;i++{
        if ctx.Err()!=nil{return}
        item,err:=d.lease(ctx)
        if errors.Is(err,pgx.ErrNoRows){return}
        if err!=nil{log.Printf("Calden push: lease failed: %v",err);return}
        if item.Token==""{
            d.finalize(ctx,item,false,false,"Device token unavailable")
            continue
        }
        plain,err:=secrets.Open(d.key,item.Token)
        if err!=nil {d.finalize(ctx,item,false,false,"Could not decrypt device token");continue}
        permanent:=false
        if err=d.send(ctx,item,plain);err!=nil {
            permanent=strings.HasPrefix(err.Error(),"invalid device token:")
            d.finalize(ctx,item,false,permanent,err.Error())
            continue
        }
        d.finalize(ctx,item,true,false,"")
    }
}

func (d *dispatcher) lease(ctx context.Context) (delivery,error){
    var item delivery
    // SKIP LOCKED allows safe future HA replicas without duplicate ownership.
    tx,err:=d.db.Begin(ctx)
    if err!=nil{return item,err}
    defer tx.Rollback(ctx)
    err=tx.QueryRow(ctx,`WITH candidate AS (
       SELECT p.notification_id,p.device_id
       FROM notification_push_deliveries p
       JOIN notifications n ON n.id=p.notification_id
       JOIN users usr ON usr.id=n.user_id
       JOIN calden_devices d ON d.id=p.device_id AND d.user_id=n.user_id
       LEFT JOIN events e ON e.id=n.event_id
       LEFT JOIN calendars cal ON cal.id=COALESCE(n.calendar_id,e.calendar_id)
       LEFT JOIN calendar_permissions perm ON perm.calendar_id=cal.id AND perm.user_id=usr.id
       LEFT JOIN family_child_profiles child ON child.user_id=usr.id
       LEFT JOIN member_notification_preferences pref ON pref.user_id=usr.id
       WHERE (p.status='pending' AND p.next_attempt_at<=now()
           OR p.status='leased' AND p.leased_until<now())
         AND d.revoked_at IS NULL AND d.enabled AND d.push_provider='fcm'
         AND d.push_token IS NOT NULL
         AND n.read_at IS NULL AND n.dismissed_at IS NULL
         AND usr.active=true
         AND COALESCE(pref.push_enabled,true)
         AND (pref.user_id IS NULL OR n.kind=ANY(pref.enabled_kinds))
         AND (n.kind NOT IN ('care_due','care_advance') OR calden_can_view_care_notification(n.care_item_id,n.user_id))
         AND (pref.quiet_start IS NULL OR NOT (
           CASE WHEN pref.quiet_start<pref.quiet_end THEN
             to_char(timezone(COALESCE((SELECT value FROM app_settings WHERE key='timezone'),'UTC'),now()),'HH24:MI')
                 >=pref.quiet_start AND
             to_char(timezone(COALESCE((SELECT value FROM app_settings WHERE key='timezone'),'UTC'),now()),'HH24:MI')
                 <pref.quiet_end
           ELSE
             to_char(timezone(COALESCE((SELECT value FROM app_settings WHERE key='timezone'),'UTC'),now()),'HH24:MI')
                 >=pref.quiet_start OR
             to_char(timezone(COALESCE((SELECT value FROM app_settings WHERE key='timezone'),'UTC'),now()),'HH24:MI')
                 <pref.quiet_end END))
         AND (cal.id IS NULL OR usr.role IN ('admin','planner') OR COALESCE(perm.can_view,false)=true)
         AND (NOT usr.is_child OR (
           child.user_id IS NOT NULL
           AND 'notifications'=ANY(child.allowed_modules)
           AND n.kind<>'bill_review'
           AND (cal.id IS NULL OR (
             cal.calendar_type<>'bill_pay'
             AND 'calendar'=ANY(child.allowed_modules)))))
       ORDER BY p.next_attempt_at
       LIMIT 1 FOR UPDATE OF p SKIP LOCKED
    ), claimed AS (
       UPDATE notification_push_deliveries p
       SET status='leased',leased_until=now()+interval '90 seconds',
           attempts=attempts+1,updated_at=now()
       FROM candidate c WHERE p.notification_id=c.notification_id
           AND p.device_id=c.device_id
       RETURNING p.notification_id,p.device_id,p.attempts
    )
    SELECT c.notification_id,c.device_id,n.user_id,d.push_token,n.title,n.message,n.kind,
        n.event_id,COALESCE(n.calendar_id,(SELECT e.calendar_id FROM events e WHERE e.id=n.event_id)),n.occurrence_start,c.attempts
    FROM claimed c JOIN calden_devices d ON d.id=c.device_id
      JOIN notifications n ON n.id=c.notification_id`).Scan(
      &item.NoticeID,&item.DeviceID,&item.RecipientID,&item.Token,&item.Title,&item.Message,
      &item.Kind,&item.EventID,&item.CalendarID,&item.OccurrenceStart,&item.Attempts)
    if err!=nil {return item,err}
    if err=tx.Commit(ctx);err!=nil{return item,err}
    return item,nil
}

func (d *dispatcher) finalize(ctx context.Context,item delivery,success,permanent bool,reason string){
    status:="pending"
    if success{status="sent"}
    if permanent || item.Attempts>=maxAttempts{status="failed"}
    backoff:=retryDelay(item.Attempts)
    _,err:=d.db.Exec(ctx,`UPDATE notification_push_deliveries SET
      status=$3::text,leased_until=NULL,updated_at=now(),
      sent_at=CASE WHEN $3::text='sent' THEN now() ELSE sent_at END,
      next_attempt_at=now()+($4::bigint * interval '1 second'),
      last_error=NULLIF($5::text,'')
      WHERE notification_id=$1 AND device_id=$2 AND status='leased'`,
      item.NoticeID,item.DeviceID,status,int64(backoff/time.Second),reason)
    if err!=nil {log.Printf("Calden push: delivery-state update failed: %v",err)}
    if permanent{
      // An invalid FCM device token must be re-registered by the client.
      _,_ = d.db.Exec(ctx,`UPDATE calden_devices
        SET push_token=NULL,push_provider='none'
        WHERE id=$1`,item.DeviceID)
    }
}

func retryDelay(attempts int) time.Duration {
    if attempts<1{attempts=1}
    if attempts>8{attempts=8}
    delay:=time.Duration(1<<uint(attempts))*15*time.Second
    if delay>2*time.Hour{return 2*time.Hour}
    return delay
}

func (d *dispatcher) oauthToken(ctx context.Context) (string,error){
    if d.accessToken!="" && time.Now().Before(d.accessUntil.Add(-90*time.Second)) {
        return d.accessToken,nil
    }
    block,_:=pem.Decode([]byte(d.credentials.PrivateKey))
    if block==nil{return "",errors.New("invalid service private key")}
    var private *rsa.PrivateKey
    if parsed,e:=x509.ParsePKCS8PrivateKey(block.Bytes);e==nil {
        var ok bool
        private,ok=parsed.(*rsa.PrivateKey)
        if !ok{return "",errors.New("service private key must be RSA")}
    }else{
        private,e=x509.ParsePKCS1PrivateKey(block.Bytes)
        if e!=nil{return "",errors.New("invalid RSA service private key")}
    }
    now:=time.Now().Unix()
    header:=map[string]string{"alg":"RS256","typ":"JWT"}
    payload:=map[string]any{"iss":d.credentials.ClientEmail,
      "scope":"https://www.googleapis.com/auth/firebase.messaging",
      "aud":d.credentials.TokenURI,"iat":now,"exp":now+3600}
    h,_:=json.Marshal(header);p,_:=json.Marshal(payload)
    claim:=base64.RawURLEncoding.EncodeToString(h)+"."+base64.RawURLEncoding.EncodeToString(p)
    sum:=sha256.Sum256([]byte(claim))
    signature,err:=rsa.SignPKCS1v15(rand.Reader,private,crypto.SHA256,sum[:])
    if err!=nil{return "",err}
    assertion:=claim+"."+base64.RawURLEncoding.EncodeToString(signature)
    form:=url.Values{"grant_type":{"urn:ietf:params:oauth:grant-type:jwt-bearer"},
      "assertion":{assertion}}
    req,err:=http.NewRequestWithContext(ctx,"POST",d.credentials.TokenURI,
        strings.NewReader(form.Encode()))
    if err!=nil{return "",err}
    req.Header.Set("Content-Type","application/x-www-form-urlencoded")
    res,err:=d.http.Do(req)
    if err!=nil{return "",err}
    defer res.Body.Close()
    if res.StatusCode!=200 {return "",fmt.Errorf("FCM OAuth returned %s",res.Status)}
    var out struct{AccessToken string `json:"access_token"`;ExpiresIn int `json:"expires_in"`}
    if json.NewDecoder(io.LimitReader(res.Body,16384)).Decode(&out)!=nil || out.AccessToken=="" {
      return "",errors.New("FCM OAuth returned no access token")
    }
    d.accessToken=out.AccessToken
    if out.ExpiresIn<180 {out.ExpiresIn=180}
    d.accessUntil=time.Now().Add(time.Duration(out.ExpiresIn)*time.Second)
    return d.accessToken,nil
}

func (d *dispatcher) send(ctx context.Context,item delivery,token string) error{
    access,err:=d.oauthToken(ctx)
    if err!=nil{return err}
    data:=map[string]string{
        "calden_type":"notification","notification_id":item.NoticeID.String(),
        "recipient_user_id":item.RecipientID.String(),
        "title":item.Title,"message":item.Message,"kind":item.Kind,
    }
    if item.EventID!=nil {data["event_id"]=item.EventID.String()}
    if item.CalendarID!=nil {data["calendar_id"]=item.CalendarID.String()}
    if item.OccurrenceStart!=nil{data["occurrence_start"]=item.OccurrenceStart.Format(time.RFC3339Nano)}
    body:=map[string]any{"message":map[string]any{
        "token":token,
        "data":data,
        "android":map[string]any{"priority":"high",
            "ttl":"86400s"},
    }}
    raw,_:=json.Marshal(body)
    endpoint:="https://fcm.googleapis.com/v1/projects/"+url.PathEscape(d.project)+"/messages:send"
    req,err:=http.NewRequestWithContext(ctx,"POST",endpoint,bytes.NewReader(raw))
    if err!=nil{return err}
    req.Header.Set("Authorization","Bearer "+access)
    req.Header.Set("Content-Type","application/json")
    res,err:=d.http.Do(req)
    if err!=nil{return err}
    defer res.Body.Close()
    if res.StatusCode>=200 && res.StatusCode<300{return nil}
    buf,_:=io.ReadAll(io.LimitReader(res.Body,4096))
    var response struct{
      Error struct{
        Status string `json:"status"`
        Details []struct{
          ErrorCode string `json:"errorCode"`
        } `json:"details"`
      } `json:"error"`
    }
    _=json.Unmarshal(buf,&response)
    invalid:=res.StatusCode==404 || response.Error.Status=="UNREGISTERED"
    for _,detail:=range response.Error.Details {
        if detail.ErrorCode=="UNREGISTERED"{invalid=true}
    }
    if invalid{return fmt.Errorf("invalid device token: FCM %s",res.Status)}
    return fmt.Errorf("FCM delivery returned %s (%s)",res.Status,response.Error.Status)
}
