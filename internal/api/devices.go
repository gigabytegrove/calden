package api

import (
    "net/http"
    "strings"

    "github.com/google/uuid"
)

type deviceInput struct {
    InstallationID string `json:"installation_id"`
    Name string `json:"name"`
    Platform string `json:"platform"`
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
    var id uuid.UUID
    err=s.db.QueryRow(r.Context(),`INSERT INTO calden_devices(user_id,installation_id,name,platform)
        VALUES($1,$2,$3,$4)
        ON CONFLICT (user_id,installation_id) DO UPDATE SET
           name=EXCLUDED.name, platform=EXCLUDED.platform, last_seen_at=now(),\n           revoked_at=NULL, enabled=true
        RETURNING id`,currentActor(r).ID,installationID,in.Name,in.Platform).Scan(&id)
    if err!=nil {writeError(w,500,"Could not register device");return}
    writeJSON(w,200,map[string]any{"id":id,"registered":true})
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
