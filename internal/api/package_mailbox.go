package api

import (
 "context"
 "errors"
 "net/http"
 "os"
 "strings"
 "time"

 "github.com/gigabytegrove/calden/internal/secrets"
 "github.com/google/uuid"
 "github.com/jackc/pgx/v5/pgxpool"
 "github.com/jackc/pgx/v5"
)

const packageIMAPSecretPrefix = "calden-package-imap-v1:"
type packageMailboxConfig struct {
 Hostname string `json:"hostname"`
 Username string `json:"username"`
 Password string `json:"password"`
 Enabled bool `json:"enabled"`
}
type packageMailboxRecord struct {
 Configured bool `json:"configured"`
 Hostname string `json:"hostname,omitempty"`
 Username string `json:"username,omitempty"`
 Enabled bool `json:"enabled"`
 LastScan *time.Time `json:"last_scan,omitempty"`
 LastError string `json:"last_error,omitempty"`
 LastImported int `json:"last_imported"`
}
func allowedPackageIMAPHost(host string) bool{
 // Custom servers require deployment-side opt-in to avoid arbitrary internal
 // network connections and credential exfiltration to user-supplied hosts.
 if host=="imap.gmail.com" {return true}
 for _,entry:=range strings.Split(os.Getenv("CALDEN_PACKAGE_IMAP_ALLOWED_HOSTS"),","){
  entry=strings.ToLower(strings.TrimSpace(entry))
  if entry!=""&&entry==host{return true}
 }
 return false
}
func (s *server) packageMailboxInfo(w http.ResponseWriter,r *http.Request){
 if !s.requirePackages(w,r){return}
 if !packageCanWrite(r){writeError(w,403,"Mailboxes require an adult account");return}
 var info packageMailboxRecord
 err:=s.db.QueryRow(r.Context(),`SELECT hostname,username,enabled,last_scan,last_error,last_imported FROM package_mailboxes WHERE owner_id=$1`,currentActor(r).ID).Scan(&info.Hostname,&info.Username,&info.Enabled,&info.LastScan,&info.LastError,&info.LastImported)
 if errors.Is(err,pgx.ErrNoRows){writeJSON(w,200,info);return}
 if err!=nil{writeError(w,500,"Cannot load mailbox settings");return}
 info.Configured=true
 writeJSON(w,200,info)
}
func (s *server) savePackageMailbox(w http.ResponseWriter,r *http.Request){
 if !s.requirePackages(w,r){return}
 if !packageCanWrite(r){writeError(w,403,"Mailboxes require an adult account");return}
 var in packageMailboxConfig
 if decode(r,&in)!=nil{writeError(w,400,"Check the mailbox settings");return}
 in.Hostname=strings.ToLower(strings.TrimSpace(in.Hostname))
 in.Username=strings.TrimSpace(in.Username)
 if !allowedPackageIMAPHost(in.Hostname)||strings.ContainsAny(in.Hostname," /\\:@")||len(in.Username)<3||len(in.Username)>254{
  writeError(w,400,"Choose a server authorized by your Calden administrator and a valid mailbox username");return
 }
 if _,err:=imapQuote(in.Username);err!=nil{writeError(w,400,"Invalid mailbox username");return}
 if len(in.Password)>1024||strings.ContainsAny(in.Password,"\r\n\x00"){writeError(w,400,"Invalid app password");return}
 var oldHost,oldUser,oldSecret string
 err:=s.db.QueryRow(r.Context(),"SELECT hostname,username,encrypted_password FROM package_mailboxes WHERE owner_id=$1",currentActor(r).ID).Scan(&oldHost,&oldUser,&oldSecret)
 if err!=nil&&!errors.Is(err,pgx.ErrNoRows){writeError(w,500,"Could not check mailbox settings");return}
 if in.Password==""&&(oldSecret==""||oldHost!=in.Hostname||oldUser!=in.Username){
  writeError(w,400,"Enter an IMAP app password when adding or changing an account");return
 }
 sealed:=oldSecret
 if in.Password!=""{
  sealed,err=secrets.Seal(s.jwtSecret,packageIMAPSecretPrefix+in.Password)
  if err!=nil{writeError(w,500,"Could not encrypt IMAP password");return}
 }
 _,err=s.db.Exec(r.Context(),`INSERT INTO package_mailboxes(owner_id,hostname,username,encrypted_password,enabled)
   VALUES($1,$2,$3,$4,$5)
   ON CONFLICT(owner_id) DO UPDATE SET hostname=EXCLUDED.hostname,username=EXCLUDED.username,
    encrypted_password=EXCLUDED.encrypted_password,enabled=EXCLUDED.enabled,
    last_uid=CASE WHEN package_mailboxes.hostname=EXCLUDED.hostname AND package_mailboxes.username=EXCLUDED.username THEN package_mailboxes.last_uid ELSE 0 END,
    uid_validity=CASE WHEN package_mailboxes.hostname=EXCLUDED.hostname AND package_mailboxes.username=EXCLUDED.username THEN package_mailboxes.uid_validity ELSE 0 END,
    next_scan=now(),last_error='',updated_at=now()`,currentActor(r).ID,in.Hostname,in.Username,sealed,in.Enabled)
 if err!=nil{writeError(w,500,"Could not save mailbox settings");return}
 s.audit(r,"update","package_mailbox",nil,"Updated package discovery mailbox",nil)
 writeJSON(w,200,map[string]any{"configured":true,"hostname":in.Hostname,"username":in.Username,"enabled":in.Enabled})
}
func (s *server) deletePackageMailbox(w http.ResponseWriter,r *http.Request){
 if !s.requirePackages(w,r){return}
 if !packageCanWrite(r){writeError(w,403,"Not permitted");return}
 _,err:=s.db.Exec(r.Context(),"DELETE FROM package_mailboxes WHERE owner_id=$1",currentActor(r).ID)
 if err!=nil{writeError(w,500,"Could not disconnect mailbox");return}
 s.audit(r,"delete","package_mailbox",nil,"Disconnected shipping mailbox",nil)
 w.WriteHeader(204)
}
func (s *server) checkPackageMailbox(w http.ResponseWriter,r *http.Request){
 if !s.requirePackages(w,r){return}
 if !packageCanWrite(r){writeError(w,403,"Not permitted");return}
 tag,err:=s.db.Exec(r.Context(),`UPDATE package_mailboxes SET next_scan=now()
 WHERE owner_id=$1 AND enabled AND (last_scan IS NULL OR last_scan < now()-interval '60 seconds')`,currentActor(r).ID)
 if err!=nil{writeError(w,500,"Could not schedule mailbox scan");return}
 if tag.RowsAffected()==0{writeError(w,409,"Enable the mailbox or wait 60 seconds between scans");return}
 writeJSON(w,202,map[string]string{"status":"queued"})
}
type packageMailboxJob struct {
 OwnerID uuid.UUID
 Hostname,Username,EncryptedPassword string
 UIDValidity,LastUID int64
}
func StartPackageMailSync(ctx context.Context,db *pgxpool.Pool,master []byte){
 go func(){
  timer:=time.NewTicker(time.Minute);defer timer.Stop()
  for{
   select{case <-ctx.Done():return;case <-timer.C:
    processDuePackageMailboxes(ctx,db,master)
   }
  }
 }()
}
func processDuePackageMailboxes(ctx context.Context,db *pgxpool.Pool,master []byte){
 rows,err:=db.Query(ctx,`WITH due AS (
   SELECT m.owner_id FROM package_mailboxes m JOIN users u ON u.id=m.owner_id
   WHERE m.enabled AND m.next_scan<=now() AND u.active AND u.role!='restricted' AND NOT u.is_child
    AND COALESCE((SELECT value='true' FROM app_settings WHERE key='packages_enabled'),false)
   ORDER BY m.next_scan LIMIT 4 FOR UPDATE OF m SKIP LOCKED
 ) UPDATE package_mailboxes m SET next_scan=now()+interval '15 minutes'
   FROM due WHERE m.owner_id=due.owner_id
   RETURNING m.owner_id,m.hostname,m.username,m.encrypted_password,m.uid_validity,m.last_uid`)
 if err!=nil{return}
 jobs:=[]packageMailboxJob{}
 for rows.Next(){var job packageMailboxJob;if rows.Scan(&job.OwnerID,&job.Hostname,&job.Username,&job.EncryptedPassword,&job.UIDValidity,&job.LastUID)==nil{jobs=append(jobs,job)}}
 rows.Close()
 for _,job:=range jobs{
  if ctx.Err()!=nil{return}
  deadline,cancel:=context.WithTimeout(ctx,55*time.Second)
  s:=&server{db:db,jwtSecret:master}
  stored,e:=secrets.Open(master,job.EncryptedPassword)
  var validity,last uint32
  var count int
  if e==nil{
   if !strings.HasPrefix(stored,packageIMAPSecretPrefix){e=errors.New("Mailbox secret format is invalid")}else{
    validity,last,count,e=scanPackageIMAP(deadline,job.Hostname,job.Username,strings.TrimPrefix(stored,packageIMAPSecretPrefix),uint32(job.UIDValidity),uint32(job.LastUID),func(item packageIMAPMessage)error{
      _,importErr:=s.storePackageMailRaw(deadline,job.OwnerID,string(item.Data))
      return importErr
    })
   }
  }
  cancel()
  detail:=""
  if e!=nil{detail="Mailbox scan failed; check credentials and TLS connectivity"}
  // Never disclose credentials or server-provided text in UI status.
  if e!=nil&&strings.Contains(e.Error(),"authentication"){detail="IMAP authentication failed; verify your app password"}
  if e!=nil&&strings.Contains(e.Error(),"TLS"){detail="IMAP TLS verification failed"}
  if e!=nil&&strings.Contains(e.Error(),"format"){detail="Saved mailbox credential is invalid; reconnect"}
  if validity==0 {validity=uint32(job.UIDValidity)}
  if last==0&&job.UIDValidity==int64(validity){last=uint32(job.LastUID)}
  _,_=db.Exec(ctx,`UPDATE package_mailboxes SET last_scan=now(),last_error=$2,last_imported=$3,
  uid_validity=$4,last_uid=$5,updated_at=now() WHERE owner_id=$1 AND encrypted_password=$6 AND enabled`,
   job.OwnerID,detail,count,int64(validity),int64(last),job.EncryptedPassword)
 }
}
