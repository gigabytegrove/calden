package api

import (
    "net/http"
    "strings"
    "time"
    "github.com/gigabytegrove/calden/internal/secrets"

    "github.com/google/uuid"
)

type deviceInput struct {
    InstallationID string `json:"installation_id"`
    Name string `json:"name"`
    Platform string `json:"platform"`
    PushProvider string `json:"push_provider"`
    PushToken string `json:"push_token"`
}

func (s *server) listDevices(w http.ResponseWriter, r *http.Request) {
    actor := currentActor(r)
    rows, err := s.db.Query(r.Context(), `SELECT id,installation_id,name,platform,push_provider,enabled,created_at,last_seen_at
        FROM calden_devices WHERE user_id=$1 AND revoked_at IS NULL ORDER BY last_seen_at DESC`, actor.ID)
    if err != nil { writeError(w, 500, "Could not load devices"); return }
    defer rows.Close()
    items := []map[string]any{}
    for rows.Next() {
        var id, installationID uuid.UUID
        var name, platform, provider string
        var enabled bool
        var created, seen any
        if err := rows.Scan(&id,&installationID,&name,&platform,&provider,&enabled,&created,&seen); err != nil {
            writeError(w, 500, "Could not read devices"); return
        }
        items = append(items, map[string]any{
            "id":id, "installation_id":installationID, "name":name, "platform":platform,
            "push_provider":provider, "enabled":enabled, "created_at":created, "last_seen_at":seen,
        })
    }
    if rows.Err()!=nil {writeError(w,500,"Could not finish loading devices");return}
    writeJSON(w,200,map[string]any{"items":items})
}

func (s *server) registerDevice(w http.ResponseWriter, r *http.Request) {
    var in deviceInput
    if decode(r,&in)!=nil { writeError(w,400,"Invalid device registration");return }
    installationID,err:=uuid.Parse(in.InstallationID)
    if err!=nil || installationID==uuid.Nil {writeError(w,400,"Invalid installation ID");return}
    in.Name=cleanText(strings.TrimSpace(in.Name),80)
    if in.Name=="" {writeError(w,400,"A device name is required");return}
    if in.Platform!="android" && in.Platform!="web" && in.Platform!="ios" {
        writeError(w,400,"Unsupported device platform");return
    }
    if in.PushProvider!="" && in.PushProvider!="none" && in.PushProvider!="fcm" {
        writeError(w,400,"Unsupported push provider");return
    }
    if in.PushProvider=="fcm" && in.Platform!="android" {
        writeError(w,400,"FCM tokens require Android");return
    }
    if in.PushProvider=="fcm" && (len(in.PushToken)<20 || len(in.PushToken)>4096) {
        writeError(w,400,"Invalid push token");return
    }
    provider:=in.PushProvider
    if provider==""{provider="none"}
    var encrypted *string
    if provider=="fcm" {
        token,e:=secrets.Seal(s.jwtSecret,in.PushToken)
        if e!=nil{writeError(w,500,"Could not secure device token");return}
        encrypted=&token
    }
    var otherOwner bool
    err=s.db.QueryRow(r.Context(),`SELECT EXISTS(
       SELECT 1 FROM calden_devices WHERE installation_id=$1
       AND user_id<>$2 AND revoked_at IS NULL)`,
       installationID,currentActor(r).ID).Scan(&otherOwner)
    if err!=nil{writeError(w,500,"Could not check device ownership");return}
    if otherOwner{writeError(w,409,"Sign out of the previous account on this device first");return}
    var id uuid.UUID
    err=s.db.QueryRow(r.Context(),`INSERT INTO calden_devices(
       user_id,installation_id,name,platform,push_provider,push_token)
       VALUES($1,$2,$3,$4,$5,$6)
       ON CONFLICT(user_id,installation_id) DO UPDATE SET
         name=EXCLUDED.name, platform=EXCLUDED.platform, last_seen_at=now(),
         push_provider=CASE WHEN EXCLUDED.push_provider='none'
          THEN calden_devices.push_provider ELSE EXCLUDED.push_provider END,
         push_token=COALESCE(EXCLUDED.push_token,calden_devices.push_token),
         revoked_at=NULL,enabled=true RETURNING id`,
         currentActor(r).ID,installationID,in.Name,in.Platform,provider,encrypted).Scan(&id)
    if err!=nil{writeError(w,500,"Could not register device");return}
    writeJSON(w,200,map[string]any{"id":id,"registered":true,"push_registered":encrypted!=nil})
}

func (s *server) revokeDevice(w http.ResponseWriter, r *http.Request) {
    id,err:=uuid.Parse(r.PathValue("id"))
    if err!=nil {writeError(w,400,"Invalid device");return}
    tag,err:=s.db.Exec(r.Context(),`UPDATE calden_devices
       SET revoked_at=now(),enabled=false,push_token=NULL,push_provider='none'
       WHERE id=$1 AND user_id=$2 AND revoked_at IS NULL`,id,currentActor(r).ID)
    if err!=nil {writeError(w,500,"Could not revoke device");return}
    if tag.RowsAffected()==0 {writeError(w,404,"Device not found");return}
    w.WriteHeader(http.StatusNoContent)
}

func (s *server) pushClientConfig(w http.ResponseWriter,r *http.Request) {
    writeJSON(w,200,map[string]any{
        "enabled":s.pushProjectID!="" && s.pushSenderID!="" &&
            s.pushAppID!="" && s.pushAPIKey!="" && s.pushAccountFile!="",
        "project_id":s.pushProjectID,"sender_id":s.pushSenderID,
        "app_id":s.pushAppID,"api_key":s.pushAPIKey,
    })
}
func (s *server) pushDiagnostics(w http.ResponseWriter,r *http.Request) {
    rows,err:=s.db.Query(r.Context(),`SELECT status,count(*)
      FROM notification_push_deliveries GROUP BY status`)
    if err!=nil{writeError(w,500,"Could not read push diagnostics");return}
    defer rows.Close()
    counts:=map[string]int{}
    for rows.Next(){var status string;var count int
      if rows.Scan(&status,&count)==nil{counts[status]=count}}
    writeJSON(w,200,map[string]any{"configured":s.pushAccountFile!="" &&
        s.pushProjectID!="","deliveries":counts})
}


// Admin-only, read-only inventory of registered device installations.
// Never expose push tokens or installation secrets in the result.
func (s *server) listHouseholdDevices(w http.ResponseWriter,r *http.Request) {
    rows,err:=s.db.Query(r.Context(),
        "SELECT d.id,d.user_id,u.display_name,d.name,d.platform,d.enabled,d.created_at,d.last_seen_at "+
        "FROM calden_devices d JOIN users u ON u.id=d.user_id "+
        "WHERE d.revoked_at IS NULL ORDER BY u.display_name,d.last_seen_at DESC")
    if err!=nil{writeError(w,500,"Could not load household devices");return}
    defer rows.Close()
    items:=[]map[string]any{}
    for rows.Next(){
        var id,userID uuid.UUID
        var userName,name,platform string
        var enabled bool
        var created,seen time.Time
        if err=rows.Scan(&id,&userID,&userName,&name,&platform,&enabled,&created,&seen);err!=nil{
            writeError(w,500,"Could not read household devices");return
        }
        items=append(items,map[string]any{
            "id":id,"user_id":userID,"user_name":userName,"name":name,
            "platform":platform,"enabled":enabled,"created_at":created,"last_seen_at":seen,
        })
    }
    if rows.Err()!=nil{writeError(w,500,"Could not finish loading household devices");return}
    writeJSON(w,200,map[string]any{"items":items})
}
