package api

import (
    "net/http"
    "strings"
    "time"

    "github.com/google/uuid"
)

type confirmationReply struct {
    OccurrenceStart time.Time `json:"occurrence_start"`
    Status string `json:"status"`
    Reason string `json:"reason"`
}

// Only the assigned recipient may respond. A request change leaves the event in
// place, notifies its creator and records the response against the occurrence.
func (s *server) respondToConfirmation(w http.ResponseWriter, r *http.Request) {
    eventID, err := uuid.Parse(r.PathValue("id"))
    if err != nil { writeError(w,400,"Invalid event ID");return }
    var in confirmationReply
    if decode(r,&in)!=nil || in.OccurrenceStart.IsZero() {
        writeError(w,400,"An occurrence start is required");return
    }
    if in.Status!="confirmed" && in.Status!="change_requested" {
        writeError(w,400,"Select Confirm or Request Change");return
    }
    in.Reason=cleanText(strings.TrimSpace(in.Reason),1000)
    if in.Status=="change_requested" && in.Reason=="" {
        writeError(w,400,"Describe the scheduling conflict");return
    }
    who:=currentActor(r)
    tx,err:=s.db.Begin(r.Context())
    if err!=nil {writeError(w,500,"Could not start confirmation");return}
    defer tx.Rollback(r.Context())
    var title string
    var creator uuid.UUID
    var permitted bool
    err=tx.QueryRow(r.Context(),`SELECT e.title,e.created_by,
        EXISTS(SELECT 1 FROM event_assignees ea WHERE ea.event_id=e.id AND ea.user_id=$2)
        FROM events e
        JOIN calendars c ON c.id=e.calendar_id
        LEFT JOIN calendar_permissions p ON p.calendar_id=c.id AND p.user_id=$2
        WHERE e.id=$1 AND e.request_confirmation=true
          AND ($3='admin' OR COALESCE(p.can_view,false))`,eventID,who.ID,who.Role).Scan(&title,&creator,&permitted)
    if err!=nil || !permitted {
        writeError(w,404,"Confirmation request not found for this user");return
    }
    _,err=tx.Exec(r.Context(),`INSERT INTO event_confirmations(event_id,user_id,occurrence_start,status,reason,responded_at)
        VALUES($1,$2,$3,$4,$5,now())
        ON CONFLICT(event_id,user_id,occurrence_start)
        DO UPDATE SET status=EXCLUDED.status,reason=EXCLUDED.reason,responded_at=now(),updated_at=now()`,
        eventID,who.ID,in.OccurrenceStart,in.Status,in.Reason)
    if err!=nil {writeError(w,500,"Could not save response");return}
    if creator!=who.ID {
        message:="An assigned member confirmed this appointment."
        kind:="event_confirmation"
        if in.Status=="change_requested" {
            message="An assigned member requested a schedule change: "+in.Reason
            kind="event_change_requested"
        }
        _,err=tx.Exec(r.Context(),`INSERT INTO notifications(user_id,event_id,kind,title,message,occurrence_start)
          VALUES($1,$2,$3,$4,$5,$6)`,creator,eventID,kind,title,message,in.OccurrenceStart)
        if err!=nil {writeError(w,500,"Could not notify scheduler");return}
    }
    if err=tx.Commit(r.Context());err!=nil {writeError(w,500,"Could not finish response");return}
    writeJSON(w,200,map[string]any{"saved":true,"status":in.Status})
}
