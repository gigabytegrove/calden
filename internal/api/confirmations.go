package api

import (
    "net/http"
    "encoding/json"
    "errors"

    "github.com/jackc/pgx/v5"
    "github.com/gigabytegrove/calden/internal/recurrence"
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
    if in.Status!="confirmed" && in.Status!="change_requested" && in.Status!="pending" {
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
    var eventStart, eventEnd time.Time
    err=tx.QueryRow(r.Context(),`SELECT e.title,e.created_by,e.starts_at,e.ends_at,
        EXISTS(SELECT 1 FROM event_assignees ea WHERE ea.event_id=e.id AND ea.user_id=$2)
        FROM events e
        JOIN calendars c ON c.id=e.calendar_id
        LEFT JOIN calendar_permissions p ON p.calendar_id=c.id AND p.user_id=$2
        WHERE e.id=$1 AND e.request_confirmation=true AND e.created_by<>$2
          AND EXISTS (SELECT 1 FROM users target WHERE target.id=$2 AND target.confirmation_enabled)
          AND ($3='admin' OR COALESCE(p.can_view,false))`,eventID,who.ID,who.Role).Scan(&title,&creator,&eventStart,&eventEnd,&permitted)
    if err!=nil || !permitted {
        writeError(w,404,"Confirmation request not found for this user");return
    }
    // Reject fabricated dates and outdated occurrences, including deleted instances.
    var frequency string
    var interval int
    var weekdaysRaw []byte
    var until *time.Time
    var count *int
    var rawRule string
    err = tx.QueryRow(r.Context(), `SELECT frequency,interval_value,weekdays,until_at,occurrence_count,COALESCE(raw_rule,'')
        FROM event_recurrence WHERE event_id=$1`, eventID).
        Scan(&frequency,&interval,&weekdaysRaw,&until,&count,&rawRule)
    switch {
    case errors.Is(err,pgx.ErrNoRows):
        if !eventStart.Equal(in.OccurrenceStart) {
            writeError(w,400,"Occurrence does not belong to this appointment");return
        }
    case err != nil:
        writeError(w,500,"Could not validate recurrence");return
    default:
        weekdays := []int{}
        if len(weekdaysRaw)>0 && json.Unmarshal(weekdaysRaw,&weekdays)!=nil {
            writeError(w,500,"Could not validate recurrence days");return
        }
        rule:=recurrence.Normalize(&recurrence.Rule{
            Frequency:frequency, Interval:interval, Weekdays:weekdays,
            Until:until, OccurrenceCount:count, Raw:rawRule,
        },eventStart)
        matches:=false
        for _, occurrence := range recurrence.Expand(eventStart,eventEnd,rule,
            in.OccurrenceStart.Add(-time.Second),in.OccurrenceStart.Add(time.Second),16) {
            if occurrence.Start.Equal(in.OccurrenceStart) { matches=true;break }
        }
        if !matches {writeError(w,400,"Occurrence does not belong to this recurring appointment");return}
        var deleted bool
        if err=tx.QueryRow(r.Context(),`SELECT EXISTS(
            SELECT 1 FROM event_occurrence_exceptions x
            WHERE x.event_id=$1 AND x.original_start=$2
              AND NOT EXISTS(SELECT 1 FROM events replacement
                  WHERE replacement.recurrence_parent_id=$1
                    AND replacement.recurrence_original_start=$2))`,
            eventID,in.OccurrenceStart).Scan(&deleted);err!=nil {
            writeError(w,500,"Could not validate occurrence exception");return
        }
        if deleted {writeError(w,409,"This occurrence was deleted");return}
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
        } else if in.Status=="pending" {
            message="An assigned member withdrew their previous response; confirmation is pending."
        }
        _,err=tx.Exec(r.Context(),`INSERT INTO notifications(user_id,event_id,kind,title,message,occurrence_start)
          VALUES($1,$2,$3,$4,$5,$6)`,creator,eventID,kind,title,message,in.OccurrenceStart)
        if err!=nil {writeError(w,500,"Could not notify scheduler");return}
    }
    if err=tx.Commit(r.Context());err!=nil {writeError(w,500,"Could not finish response");return}
    writeJSON(w,200,map[string]any{"saved":true,"status":in.Status})
}

func (s *server) getEventConfirmations(w http.ResponseWriter, r *http.Request) {
    eventID, err := uuid.Parse(r.PathValue("id"))
    if err != nil { writeError(w, 400, "Invalid event ID"); return }
    actor := currentActor(r)
    var requested bool
    var creator uuid.UUID
    err = s.db.QueryRow(r.Context(), `SELECT e.request_confirmation,e.created_by FROM events e
        JOIN calendars c ON c.id=e.calendar_id
        LEFT JOIN calendar_permissions p ON p.calendar_id=c.id AND p.user_id=$2
        WHERE e.id=$1 AND ($3='admin' OR COALESCE(p.can_view,false))`,
        eventID,actor.ID,actor.Role).Scan(&requested,&creator)
    if err != nil { writeError(w,404,"Event not found"); return }

    assignees, err := s.db.Query(r.Context(), `SELECT a.user_id,u.display_name FROM event_assignees a
        JOIN users u ON u.id=a.user_id WHERE a.event_id=$1 AND u.active=true ORDER BY u.display_name`, eventID)
    if err != nil {writeError(w,500,"Could not load assignees");return}
    defer assignees.Close()
    people := []map[string]any{}
    for assignees.Next() {
        var id uuid.UUID
        var name string
        if err=assignees.Scan(&id,&name);err!=nil {writeError(w,500,"Could not read assignees");return}
        people=append(people,map[string]any{"user_id":id,"display_name":name})
    }
    if assignees.Err()!=nil {writeError(w,500,"Could not load assignees");return}
    var occurrence time.Time
    raw:=r.URL.Query().Get("occurrence_start")
    if raw!="" {
        occurrence,err=time.Parse(time.RFC3339,raw)
        if err!=nil {writeError(w,400,"Invalid occurrence start");return}
    } else {
        err=s.db.QueryRow(r.Context(),`SELECT starts_at FROM events WHERE id=$1`,eventID).Scan(&occurrence)
        if err!=nil {writeError(w,404,"Event not found");return}
    }
    rows,err:=s.db.Query(r.Context(),`SELECT user_id,status,reason,responded_at FROM event_confirmations
        WHERE event_id=$1 AND occurrence_start=$2`,eventID,occurrence)
    if err!=nil {writeError(w,500,"Could not load confirmations");return}
    defer rows.Close()
    responses:=map[string]any{}
    for rows.Next() {
        var id uuid.UUID
        var status,reason string
        var responded *time.Time
        if err=rows.Scan(&id,&status,&reason,&responded);err!=nil {writeError(w,500,"Could not read confirmations");return}
        responses[id.String()]=map[string]any{"status":status,"reason":reason,"responded_at":responded}
    }
    if rows.Err()!=nil {writeError(w,500,"Could not finish confirmations");return}
    writeJSON(w,200,map[string]any{"requested":requested,"creator_user_id":creator,"occurrence_start":occurrence,
        "assignees":people,"responses":responses,"current_user_id":actor.ID})
}
