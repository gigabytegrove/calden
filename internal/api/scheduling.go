package api

import (
    "encoding/json"
    "errors"
    "net/http"
    "strings"
    "time"

    "github.com/google/uuid"
    "github.com/jackc/pgx/v5"
    "github.com/gigabytegrove/calden/internal/recurrence"
)

type scheduleInput struct {
    CalendarID uuid.UUID `json:"calendar_id"`
    ExcludeEventID *uuid.UUID `json:"exclude_event_id,omitempty"`
    StartsAt time.Time `json:"starts_at"`
    EndsAt time.Time `json:"ends_at"`
    AssigneeIDs []uuid.UUID `json:"assignee_ids"`
    Recurrence *recurrence.Rule `json:"recurrence,omitempty"`
}
type busyInput struct {
    StartsAt time.Time `json:"starts_at"`
    EndsAt time.Time `json:"ends_at"`
    Label string `json:"label"`
}

// Half-open intervals: a meeting ending at 2 PM never conflicts with one
// starting at 2 PM. This is shared by repeated and one-time appointments.
func scheduleOverlap(aStart,aEnd,bStart,bEnd time.Time) bool {
    return aStart.Before(bEnd) && aEnd.After(bStart)
}

func (s *server) listBusyBlocks(w http.ResponseWriter,r *http.Request) {
    who:=currentActor(r)
    rows,err:=s.db.Query(r.Context(),`SELECT id,starts_at,ends_at,label
        FROM scheduling_busy_blocks WHERE user_id=$1 AND ends_at>now()-interval '30 days'
        ORDER BY starts_at LIMIT 300`,who.ID)
    if err!=nil {writeError(w,500,"Could not load busy time");return}
    defer rows.Close()
    out:=[]map[string]any{}
    for rows.Next(){
        var id uuid.UUID
        var from,to time.Time
        var label string
        if err=rows.Scan(&id,&from,&to,&label);err!=nil{writeError(w,500,"Could not load busy time");return}
        out=append(out,map[string]any{"id":id,"starts_at":from,"ends_at":to,"label":label})
    }
    if rows.Err()!=nil {writeError(w,500,"Could not finish loading busy time");return}
    writeJSON(w,200,map[string]any{"items":out})
}
func (s *server) createBusyBlock(w http.ResponseWriter,r *http.Request) {
    var in busyInput
    if decode(r,&in)!=nil || in.StartsAt.IsZero() || !in.EndsAt.After(in.StartsAt) ||
        in.EndsAt.Sub(in.StartsAt)>14*24*time.Hour {
        writeError(w,400,"Enter a busy start and end, up to 14 days apart");return
    }
    if in.StartsAt.Before(time.Now().AddDate(-1,0,0)) ||
        in.StartsAt.After(time.Now().AddDate(2,0,0)) {
        writeError(w,400,"Busy time must be within the supported two-year planning period");return
    }
    label:=cleanText(strings.TrimSpace(in.Label),120)
    var id uuid.UUID
    err:=s.db.QueryRow(r.Context(),`INSERT INTO scheduling_busy_blocks(
        user_id,starts_at,ends_at,label) VALUES($1,$2,$3,$4) RETURNING id`,
        currentActor(r).ID,in.StartsAt,in.EndsAt,label).Scan(&id)
    if err!=nil{writeError(w,500,"Could not save busy time");return}
    writeJSON(w,201,map[string]any{"id":id})
}
func (s *server) deleteBusyBlock(w http.ResponseWriter,r *http.Request) {
    id,err:=uuid.Parse(r.PathValue("id"))
    if err!=nil{writeError(w,400,"Invalid busy block");return}
    tag,err:=s.db.Exec(r.Context(),`DELETE FROM scheduling_busy_blocks
        WHERE id=$1 AND user_id=$2`,id,currentActor(r).ID)
    if err!=nil{writeError(w,500,"Could not delete busy time");return}
    if tag.RowsAffected()==0 {writeError(w,404,"Busy block not found");return}
    w.WriteHeader(http.StatusNoContent)
}
type existingSchedule struct {
    ID,CalendarID uuid.UUID
    StartsAt,EndsAt time.Time
    Status string
    AssigneeIDs []uuid.UUID
    Rule *recurrence.Rule
    IsOverride bool
}
// Preview-only. Never silently prevent the user from saving an appointment.
// Details of other users' private busy blocks are never exposed.
func (s *server) checkScheduleConflicts(w http.ResponseWriter,r *http.Request) {
    who:=currentActor(r)
    var in scheduleInput
    if decode(r,&in)!=nil || in.CalendarID==uuid.Nil || in.StartsAt.IsZero() ||
        !in.EndsAt.After(in.StartsAt) || in.EndsAt.Sub(in.StartsAt)>14*24*time.Hour {
        writeError(w,400,"Check the proposed appointment date, time and calendar");return
    }
    if len(in.AssigneeIDs)>100 {writeError(w,400,"Too many assigned people");return}
    recipients:=map[uuid.UUID]bool{}
    for _,person:=range in.AssigneeIDs {
        if person==uuid.Nil {writeError(w,400,"Invalid household member");return}
        if who.Role!="admin" && who.Role!="planner" && person!=who.ID {
            writeError(w,403,"Only planners can check other household members' availability");return
        }
        recipients[person]=true
    }
    // No cross-calendar enumeration for members without permission.
    if who.Role!="admin" && who.Role!="planner" && !s.canEditCalendar(r,in.CalendarID) {
        writeError(w,403,"You cannot check this calendar");return
    }
    if in.Recurrence!=nil {
        in.Recurrence=recurrence.Normalize(in.Recurrence,in.StartsAt)
        if err:=recurrence.Validate(in.Recurrence);err!=nil{writeError(w,400,err.Error());return}
    }
    horizon:=in.StartsAt.AddDate(0,0,90)
    if in.Recurrence==nil { horizon=in.EndsAt }
    if horizon.Before(in.EndsAt){horizon=in.EndsAt}
    proposed:=recurrence.Expand(in.StartsAt,in.EndsAt,in.Recurrence,in.StartsAt,horizon,100)
    if len(proposed)==0 {writeJSON(w,200,map[string]any{"conflicts":[]any{}});return}

    rows,err:=s.db.Query(r.Context(),`SELECT e.id,e.calendar_id,e.starts_at,e.ends_at,e.status,
        e.recurrence_parent_id IS NOT NULL,
        er.frequency,er.interval_value,er.weekdays,er.until_at,er.occurrence_count,
        COALESCE(er.raw_rule,'')
        FROM events e JOIN calendars c ON c.id=e.calendar_id
        LEFT JOIN event_recurrence er ON er.event_id=e.id
        WHERE e.starts_at<$1 AND (er.event_id IS NOT NULL OR e.ends_at>$2)
          AND (er.until_at IS NULL OR er.until_at >= $2)
          AND e.status!='cancelled' AND c.calendar_type!='bill_pay'
        ORDER BY e.starts_at LIMIT 1500`,horizon,in.StartsAt.Add(-14*24*time.Hour))
    if err!=nil {writeError(w,500,"Could not check appointments");return}
    existing:=[]existingSchedule{}
    for rows.Next(){
        var e existingSchedule
        var frequency *string
        var interval,count *int
        var weekdays []byte
        var until *time.Time
        var rawRule string
        if err=rows.Scan(&e.ID,&e.CalendarID,&e.StartsAt,&e.EndsAt,&e.Status,&e.IsOverride,
            &frequency,&interval,&weekdays,&until,&count,&rawRule);err!=nil{break}
        if frequency!=nil && interval!=nil {
            days:=[]int{}
            _=json.Unmarshal(weekdays,&days)
            e.Rule=recurrence.Normalize(&recurrence.Rule{Frequency:*frequency,Interval:*interval,
                Weekdays:days,Until:until,OccurrenceCount:count,Raw:rawRule},e.StartsAt)
        }
        existing=append(existing,e)
    }
    rowsErr:=rows.Err()
    rows.Close()
    if err!=nil || rowsErr!=nil {writeError(w,500,"Could not read schedule");return}
    if len(existing)==1500 {writeError(w,409,"Schedule too large to check completely; narrow the date range");return}

    // Assignees are resolved in one DB operation; no member's busy label or
    // confidential calendar details are returned by this endpoint.
    members,err:=s.db.Query(r.Context(),`SELECT ea.event_id,ea.user_id
        FROM event_assignees ea JOIN users u ON u.id=ea.user_id
        WHERE u.active=true`)
    if err!=nil{writeError(w,500,"Could not check assignees");return}
    peopleByEvent:=map[uuid.UUID][]uuid.UUID{}
    for members.Next(){
        var eventID,personID uuid.UUID
        if members.Scan(&eventID,&personID)==nil {
            peopleByEvent[eventID]=append(peopleByEvent[eventID],personID)
        }
    }
    membersErr:=members.Err();members.Close()
    if membersErr!=nil{writeError(w,500,"Could not check assignees");return}
    // Preload series exceptions so rescheduled/cancelled occurrences don't
    // report their removed original dates as busy.
    removed:=map[uuid.UUID]map[string]bool{}
    exceptions,err:=s.db.Query(r.Context(),`SELECT event_id,original_start FROM event_occurrence_exceptions`)
    if err!=nil{writeError(w,500,"Could not check recurrence exceptions");return}
    for exceptions.Next(){
        var id uuid.UUID
        var date time.Time
        if exceptions.Scan(&id,&date)==nil {
            if removed[id]==nil{removed[id]=map[string]bool{}}
            removed[id][date.UTC().Format(time.RFC3339Nano)]=true
        }
    }
    exceptionsErr:=exceptions.Err();exceptions.Close()
    if exceptionsErr!=nil{writeError(w,500,"Could not check recurrence exceptions");return}
    matches:=[]map[string]any{}
    seen:=map[string]bool{}
    for _,wanted:=range proposed {
        for _,current:=range existing {
            if in.ExcludeEventID!=nil && current.ID==*in.ExcludeEventID {continue}
            common:=current.CalendarID==in.CalendarID
            for _,person:=range peopleByEvent[current.ID] {if recipients[person] {common=true;break}}
            if !common{continue}
            for _,actual:=range recurrence.Expand(current.StartsAt,current.EndsAt,current.Rule,
                wanted.Start,wanted.End,100) {
                if removed[current.ID][actual.Start.UTC().Format(time.RFC3339Nano)] {continue}
                if !scheduleOverlap(wanted.Start,wanted.End,actual.Start,actual.End){continue}
                key:=current.ID.String()+"/"+actual.Start.Format(time.RFC3339Nano)+"/"+wanted.Start.Format(time.RFC3339Nano)
                if seen[key]{continue}
                seen[key]=true
                matches=append(matches,map[string]any{
                    "kind":"appointment","starts_at":actual.Start,"ends_at":actual.End,
                    "proposed_start":wanted.Start,
                    // Calendar details/titles intentionally not included.
                    "private":true,
                })
                if len(matches)>=100 {break}
            }
            if len(matches)>=100{break}
        }
        if len(matches)>=100{break}
    }
    if len(recipients)>0 {
        ids:=make([]uuid.UUID,0,len(recipients))
        for id:=range recipients{ids=append(ids,id)}
        holds,err:=s.db.Query(r.Context(),`SELECT starts_at,ends_at FROM scheduling_busy_blocks
            WHERE user_id=ANY($1::uuid[]) AND starts_at<$2 AND ends_at>$3
            ORDER BY starts_at LIMIT 250`,ids,horizon,in.StartsAt)
        if err!=nil{writeError(w,500,"Could not check private busy time");return}
        for holds.Next(){
            var start,end time.Time
            if holds.Scan(&start,&end)!=nil{continue}
            for _,wanted:=range proposed {
                if scheduleOverlap(wanted.Start,wanted.End,start,end) {
                    matches=append(matches,map[string]any{
                        "kind":"private_busy","starts_at":start,"ends_at":end,
                        "proposed_start":wanted.Start,"private":true,
                    })
                    break
                }
            }
        }
        holdsErr:=holds.Err();holds.Close()
        if holdsErr!=nil{writeError(w,500,"Could not check private busy time");return}
    }
    writeJSON(w,200,map[string]any{"conflicts":matches,"checked_until":horizon})
}
// Keep package-level pgx import used only for typed ErrNoRows if later needed.
var _ = errors.Is
var _ = pgx.ErrNoRows
