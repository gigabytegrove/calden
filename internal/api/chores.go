package api

import (
    "errors"
    "net/http"
    "strings"
    "time"

    "github.com/google/uuid"
    "github.com/jackc/pgx/v5"
)

// Every member has independent progress for every occurrence; a series remains
// one editable chore. The legacy single-assignee fields remain API-compatible.
type choreInput struct {
    Kind string `json:"kind"`
    RewardPoints *int `json:"reward_points,omitempty"`
    Title string `json:"title"`
    Notes string `json:"notes"`
    DueDate string `json:"due_date"`
    AssignedTo string `json:"assigned_to"`
    AssigneeIDs []string `json:"assignee_ids"`
    RecurrenceRule string `json:"recurrence_rule"`
    RecurrenceUntil string `json:"recurrence_until"`
}
type choreStatusInput struct {
    Status string `json:"status"`
    AssigneeID string `json:"assignee_id"`
    OccurrenceDate string `json:"occurrence_date"`
}
type choreSeries struct {
    ID, Primary, Creator uuid.UUID
    Title, Notes, Name, Initials, Status, Rule, Kind string
    Avatar *string
    RewardPoints int
    Start time.Time
    Until *time.Time
    Created, Updated time.Time
}
type choreMember struct {
    ID uuid.UUID
    Name, Initials string
    Avatar *string
}
type choreProgress struct {
    Status string
    CompletedAt *time.Time
    CompletedBy *uuid.UUID
}
var choreRules = map[string]bool{
    "none":true,"daily":true,"weekly":true,"biweekly":true,
    "monthly":true,"yearly":true,
}
func choreDate(value string) (time.Time,error) {
    date,err:=time.Parse("2006-01-02",value)
    if err!=nil || date.Format("2006-01-02")!=value {
        return time.Time{},errors.New("Invalid date")
    }
    return date,nil
}
func daysBetween(a,b time.Time) int {
    return int(b.Sub(a).Hours()/24)
}
func choreDueOn(start,day time.Time,rule string) bool {
    if day.Before(start) {return false}
    delta:=daysBetween(start,day)
    switch rule {
    case "none": return delta==0
    case "daily": return true
    case "weekly": return delta%7==0
    case "biweekly": return delta%14==0
    case "monthly":
        months:=(day.Year()-start.Year())*12+int(day.Month())-int(start.Month())
        if months<0{return false}
        last:=time.Date(day.Year(),day.Month()+1,0,0,0,0,0,time.UTC).Day()
        expected:=start.Day()
        if expected>last {expected=last}
        return day.Day()==expected
    case "yearly":
        if day.Month()!=start.Month(){return false}
        last:=time.Date(day.Year(),day.Month()+1,0,0,0,0,0,time.UTC).Day()
        expected:=start.Day()
        if expected>last {expected=last}
        return day.Day()==expected
    }
    return false
}
func choreKey(choreID,memberID uuid.UUID,day string) string {
    return choreID.String()+"/"+memberID.String()+"/"+day
}
func canManageChores(role string) bool {return role=="admin" || role=="planner"}

func (s *server) choresEnabled(r *http.Request) (bool,error) {
    var enabled bool
    err:=s.db.QueryRow(r.Context(),"SELECT COALESCE((SELECT value='true' FROM app_settings WHERE key='chores_enabled'),false)").Scan(&enabled)
    return enabled,err
}
// Tasks and chores are separate household features despite the shared API.
func (s *server) enabledWorkKinds(r *http.Request) (tasks,chores bool,err error) {
    err=s.db.QueryRow(r.Context(),
        "SELECT COALESCE((SELECT value='true' FROM app_settings WHERE key='tasks_enabled'),false),COALESCE((SELECT value='true' FROM app_settings WHERE key='chores_enabled'),false)").Scan(&tasks,&chores)
    return
}
func (s *server) requireChores(w http.ResponseWriter,r *http.Request) bool {
    tasks,chores,err:=s.enabledWorkKinds(r)
    if err!=nil {writeError(w,500,"Could not load optional feature settings");return false}
    if !tasks&&!chores {writeError(w,403,"Tasks and Chores are turned off by your household administrator");return false}
    return true
}
func childWorkModule(kind string)string{
    if kind=="task"{return "tasks"}
    return "chores"
}
func (s *server) requireWorkKind(w http.ResponseWriter,r *http.Request,kind string) bool {
    tasks,chores,err:=s.enabledWorkKinds(r)
    if err!=nil {writeError(w,500,"Could not load optional feature settings");return false}
    if (kind=="task"&&tasks)||(kind=="chore"&&chores){return true}
    writeError(w,403,"This household feature is turned off by your administrator")
    return false
}
func (s *server) saveChoresEnabled(w http.ResponseWriter,r *http.Request) {
    var input struct {Enabled *bool `json:"enabled"`}
    if decode(r,&input)!=nil || input.Enabled==nil {writeError(w,400,"Choose whether Tasks & Chores is enabled");return}
    value:="false";if *input.Enabled {value="true"}
    _,err:=s.db.Exec(r.Context(),"INSERT INTO app_settings(key,value) VALUES('chores_enabled',$1) ON CONFLICT(key) DO UPDATE SET value=EXCLUDED.value,updated_at=now()",value)
    if err!=nil {writeError(w,500,"Could not save Tasks & Chores setting");return}
    s.audit(r,"update","settings",nil,"Changed Tasks & Chores availability",map[string]any{"enabled":*input.Enabled})
    writeJSON(w,200,map[string]bool{"enabled":*input.Enabled})
}

func validateChore(input choreInput) (choreInput,[]uuid.UUID,time.Time,*time.Time,string) {
    if input.Kind=="" {input.Kind="chore"} // older Android builds create chores
    if input.Kind!="task"&&input.Kind!="chore" {return input,nil,time.Time{},nil,"Choose Task or Chore"}
    if input.RewardPoints!=nil && (*input.RewardPoints<0||*input.RewardPoints>10000) {return input,nil,time.Time{},nil,"Choose 0–10,000 reward points"}
    if input.Kind=="task" && input.RewardPoints!=nil && *input.RewardPoints>0 {return input,nil,time.Time{},nil,"Points are only available for chores"}
    input.Title=cleanText(strings.TrimSpace(input.Title),200)
    input.Notes=cleanText(strings.TrimSpace(input.Notes),4000)
    if input.Title=="" {return input,nil,time.Time{},nil,"Enter a task or chore name"}
    start,err:=choreDate(input.DueDate)
    if err!=nil {return input,nil,time.Time{},nil,"Choose a valid due date"}
    if input.RecurrenceRule=="" {input.RecurrenceRule="none"}
    if !choreRules[input.RecurrenceRule] {return input,nil,time.Time{},nil,"Choose a valid repeat schedule"}
    var until *time.Time
    if strings.TrimSpace(input.RecurrenceUntil)!="" {
        end,e:=choreDate(input.RecurrenceUntil)
        if e!=nil||end.Before(start){return input,nil,time.Time{},nil,"Repeat end date must be on or after the first due date"}
        if input.RecurrenceRule=="none" {return input,nil,time.Time{},nil,"A repeat end date requires a repeating chore"}
        until=&end
    }
    people:=input.AssigneeIDs
    if len(people)==0 && input.AssignedTo!="" {people=[]string{input.AssignedTo}}
    if len(people)==0||len(people)>30{return input,nil,time.Time{},nil,"Choose 1–30 household members"}
    ids:=make([]uuid.UUID,0,len(people))
    seen:=map[uuid.UUID]bool{}
    for _,value:=range people {
        id,e:=uuid.Parse(strings.TrimSpace(value))
        if e!=nil||id==uuid.Nil||seen[id]{return input,nil,time.Time{},nil,"Choose unique, valid household members"}
        seen[id]=true
        ids=append(ids,id)
    }
    return input,ids,start,until,""
}

func (s *server) listChores(w http.ResponseWriter,r *http.Request) {
    if !s.requireChores(w,r){return}
    tasksOn,choresOn,settingsErr:=s.enabledWorkKinds(r)
    if settingsErr!=nil{writeError(w,500,"Could not load optional feature settings");return}
    rows,err:=s.db.Query(r.Context(),"SELECT ch.id,ch.title,ch.notes,ch.due_date,ch.assigned_to,u.display_name,u.initials,u.avatar_url,ch.created_by,ch.status,ch.recurrence_rule,ch.recurrence_until,ch.created_at,ch.updated_at,ch.kind,ch.reward_points FROM chores ch JOIN users u ON u.id=ch.assigned_to WHERE (ch.kind='task' AND $1) OR (ch.kind='chore' AND $2) ORDER BY ch.due_date,ch.created_at DESC",tasksOn,choresOn)
    if err!=nil {writeError(w,500,"Could not load tasks and chores");return}
    series:=[]choreSeries{}
    for rows.Next(){
        var ch choreSeries
        if err=rows.Scan(&ch.ID,&ch.Title,&ch.Notes,&ch.Start,&ch.Primary,&ch.Name,&ch.Initials,&ch.Avatar,&ch.Creator,&ch.Status,&ch.Rule,&ch.Until,&ch.Created,&ch.Updated,&ch.Kind,&ch.RewardPoints);err!=nil{break}
        series=append(series,ch)
    }
    if err==nil {err=rows.Err()}
    rows.Close()
    if err!=nil {writeError(w,500,"Could not read tasks and chores");return}
    members:=map[uuid.UUID][]choreMember{}
    rows,err=s.db.Query(r.Context(),"SELECT a.chore_id,u.id,u.display_name,u.initials,u.avatar_url FROM chore_assignees a JOIN users u ON u.id=a.user_id ORDER BY u.display_name")
    if err!=nil {writeError(w,500,"Could not load chore assignments");return}
    for rows.Next(){
        var choreID uuid.UUID
        var p choreMember
        if err=rows.Scan(&choreID,&p.ID,&p.Name,&p.Initials,&p.Avatar);err!=nil{break}
        members[choreID]=append(members[choreID],p)
    }
    if err==nil{err=rows.Err()}
    rows.Close()
    if err!=nil {writeError(w,500,"Could not read chore assignments");return}
    progress:=map[string]choreProgress{}
    rows,err=s.db.Query(r.Context(),"SELECT chore_id,user_id,to_char(occurrence_date,'YYYY-MM-DD'),status,completed_at,completed_by FROM chore_progress")
    if err!=nil {writeError(w,500,"Could not load chore progress");return}
    for rows.Next(){
        var choreID,memberID uuid.UUID
        var day string
        var p choreProgress
        if err=rows.Scan(&choreID,&memberID,&day,&p.Status,&p.CompletedAt,&p.CompletedBy);err!=nil{break}
        progress[choreKey(choreID,memberID,day)]=p
    }
    if err==nil{err=rows.Err()}
    rows.Close()
    if err!=nil {writeError(w,500,"Could not read chore progress");return}
    // Use household timezone for the date window; dates themselves remain date-only.
    tz:="UTC"
    _=s.db.QueryRow(r.Context(),"SELECT COALESCE((SELECT value FROM app_settings WHERE key='timezone'),'UTC')").Scan(&tz)
    loc,e:=time.LoadLocation(tz);if e!=nil{loc=time.UTC}
    now:=time.Now().In(loc)
    today,_:=choreDate(now.Format("2006-01-02"))
    past:=today.AddDate(0,0,-21)
    future:=today.AddDate(0,0,45)
    out:=[]map[string]any{}
    for _,ch:=range series{
        if currentActor(r).IsChild && !childModuleAllowed(currentActor(r).ChildModules,childWorkModule(ch.Kind)){continue}
        first,last:=ch.Start,ch.Start
        if ch.Rule!="none" {
            if first.Before(past){first=past}
            last=future
            if ch.Until!=nil&&ch.Until.Before(last){last=*ch.Until}
        }
        if currentActor(r).IsChild {
            assigned:=false
            for _,person:=range members[ch.ID] {
                if person.ID==currentActor(r).ID {assigned=true;break}
            }
            if !assigned && ch.Primary!=currentActor(r).ID {continue}
        }
        for day:=first;!day.After(last);day=day.AddDate(0,0,1){
            if !choreDueOn(ch.Start,day,ch.Rule){continue}
            date:=day.Format("2006-01-02")
            assigned:=members[ch.ID]
            if len(assigned)==0 {assigned=[]choreMember{{ID:ch.Primary,Name:ch.Name,Initials:ch.Initials,Avatar:ch.Avatar}}}
            if currentActor(r).IsChild {
                own:=make([]choreMember,0,1)
                for _,person:=range assigned{
                    if person.ID==currentActor(r).ID {own=append(own,person)}
                }
                if len(own)==0 {continue}
                assigned=own
            }
            people:=make([]map[string]any,0,len(assigned))
            completed,started:=0,0
            var completedAt *time.Time
            for _,person:=range assigned {
                p,found:=progress[choreKey(ch.ID,person.ID,date)]
                if !found {p.Status="pending"}
                if p.Status=="done"{completed++;completedAt=p.CompletedAt}
                if p.Status=="in_progress"{started++}
                people=append(people,map[string]any{
                    "id":person.ID,"name":person.Name,"initials":person.Initials,
                    "avatar_url":person.Avatar,"status":p.Status,"completed_at":p.CompletedAt,
                })
            }
            status:="pending"
            if completed==len(assigned){status="done"}else if completed>0||started>0{status="in_progress"}
            var finished any
            if status=="done"{finished=completedAt}
            out=append(out,map[string]any{
                "id":ch.ID,"kind":ch.Kind,"title":ch.Title,"notes":ch.Notes,"reward_points":ch.RewardPoints,
                "due_date":date,"occurrence_date":date,
                "series_due_date":ch.Start.Format("2006-01-02"),
                "assigned_to":ch.Primary,"assignee_name":ch.Name,"assignee_initials":ch.Initials,
                "assignee_avatar_url":ch.Avatar,"assignees":people,
                "assignee_ids":func() []uuid.UUID {v:=make([]uuid.UUID,0,len(assigned));for _,p:=range assigned {v=append(v,p.ID)};return v}(),
                "recurrence_rule":ch.Rule,"recurrence_until":func()string{if ch.Until!=nil{return ch.Until.Format("2006-01-02")};return ""}(),
                "created_by":ch.Creator,"status":status,"completed_at":finished,
                "created_at":ch.Created,"updated_at":ch.Updated,
            })
        }
    }
    writeJSON(w,200,map[string]any{"items":out})
}

func (s *server) createChore(w http.ResponseWriter,r *http.Request) {
    if !s.requireChores(w,r){return}
    if !canManageChores(currentActor(r).Role){writeError(w,403,"Only administrators and planners can assign chores");return}
    var input choreInput
    if decode(r,&input)!=nil{writeError(w,400,"Check the task details");return}
    input,people,_,until,msg:=validateChore(input)
    if msg!=""{writeError(w,400,msg);return}
    if !s.requireWorkKind(w,r,input.Kind){return}
    tx,err:=s.db.Begin(r.Context())
    if err!=nil{writeError(w,500,"Could not create chore");return}
    defer tx.Rollback(r.Context())
    var active int
    err=tx.QueryRow(r.Context(),"SELECT COUNT(*) FROM users WHERE id=ANY($1::uuid[]) AND active=true",people).Scan(&active)
    if err!=nil||active!=len(people){writeError(w,400,"Choose active household members");return}
    var id uuid.UUID
    err=tx.QueryRow(r.Context(),"INSERT INTO chores(title,notes,due_date,assigned_to,created_by,recurrence_rule,recurrence_until,kind,reward_points) VALUES($1,$2,$3::date,$4,$5,$6,$7,$8,COALESCE($9,0)) RETURNING id",input.Title,input.Notes,input.DueDate,people[0],currentActor(r).ID,input.RecurrenceRule,until,input.Kind,input.RewardPoints).Scan(&id)
    if err!=nil{writeError(w,500,"Could not create chore");return}
    for _,person:=range people{
        _,err=tx.Exec(r.Context(),"INSERT INTO chore_assignees(chore_id,user_id) VALUES($1,$2)",id,person)
        if err!=nil{writeError(w,500,"Could not assign chore");return}
    }
    if err=tx.Commit(r.Context());err!=nil{writeError(w,500,"Could not save chore");return}
    s.audit(r,"create","chore",&id,"Created household chore",map[string]any{"assignee_ids":people,"recurrence":input.RecurrenceRule})
    writeJSON(w,201,map[string]any{"id":id})
}

func (s *server) updateChore(w http.ResponseWriter,r *http.Request) {
    if !s.requireChores(w,r){return}
    if !canManageChores(currentActor(r).Role){writeError(w,403,"Only administrators and planners can edit chores");return}
    id,err:=uuid.Parse(r.PathValue("id"));if err!=nil{writeError(w,400,"Invalid chore");return}
    var input choreInput
    if decode(r,&input)!=nil{writeError(w,400,"Check the task details");return}
    var existingKind string
    if err=s.db.QueryRow(r.Context(),"SELECT kind FROM chores WHERE id=$1",id).Scan(&existingKind);errors.Is(err,pgx.ErrNoRows){writeError(w,404,"Task or chore not found");return}
    if err!=nil{writeError(w,500,"Could not load task or chore");return}
    if !s.requireWorkKind(w,r,existingKind){return}
    if input.Kind=="" {input.Kind=existingKind} // keep older client updates compatible
    input,people,_,until,msg:=validateChore(input)
    if msg!=""{writeError(w,400,msg);return}
    if !s.requireWorkKind(w,r,input.Kind){return}
    tx,err:=s.db.Begin(r.Context());if err!=nil{writeError(w,500,"Could not edit chore");return}
    defer tx.Rollback(r.Context())
    _,err=tx.Exec(r.Context(),"SELECT 1 FROM chores WHERE id=$1 FOR UPDATE",id)
    if err!=nil {writeError(w,500,"Could not lock chore for editing");return}
    var active int
    err=tx.QueryRow(r.Context(),"SELECT COUNT(*) FROM users WHERE id=ANY($1::uuid[]) AND active=true",people).Scan(&active)
    if err!=nil||active!=len(people){writeError(w,400,"Choose active household members");return}
    var changed bool
    err=tx.QueryRow(r.Context(),"UPDATE chores SET title=$2,notes=$3,due_date=$4::date,assigned_to=$5,recurrence_rule=$6,recurrence_until=$7,kind=$8,reward_points=CASE WHEN $8='task' THEN 0 ELSE COALESCE($9,reward_points) END,updated_at=now(),status=CASE WHEN due_date IS DISTINCT FROM $4::date OR recurrence_rule IS DISTINCT FROM $6 OR assigned_to IS DISTINCT FROM $5 THEN 'pending' ELSE status END,completed_at=CASE WHEN due_date IS DISTINCT FROM $4::date OR recurrence_rule IS DISTINCT FROM $6 OR assigned_to IS DISTINCT FROM $5 THEN NULL ELSE completed_at END,completed_by=CASE WHEN due_date IS DISTINCT FROM $4::date OR recurrence_rule IS DISTINCT FROM $6 OR assigned_to IS DISTINCT FROM $5 THEN NULL ELSE completed_by END WHERE id=$1 RETURNING true",id,input.Title,input.Notes,input.DueDate,people[0],input.RecurrenceRule,until,input.Kind,input.RewardPoints).Scan(&changed)
    if errors.Is(err,pgx.ErrNoRows){writeError(w,404,"Chore not found");return}
    if err!=nil{writeError(w,500,"Could not update chore");return}
    var protectedAssignee bool
    err=tx.QueryRow(r.Context(),`SELECT EXISTS(SELECT 1 FROM family_chore_reviews
       WHERE chore_id=$1 AND decision='approved'
       AND NOT (user_id = ANY($2::uuid[])))`,id,people).Scan(&protectedAssignee)
    if err!=nil {writeError(w,500,"Could not verify prior reward approvals");return}
    if protectedAssignee {writeError(w,409,"An approved reward exists for a removed assignee; keep them assigned to preserve history");return}
    // Keeping unchanged assignments preserves their occurrence history.
    _,err=tx.Exec(r.Context(),"DELETE FROM chore_assignees WHERE chore_id=$1 AND NOT (user_id=ANY($2::uuid[]))",id,people)
    if err!=nil{writeError(w,500,"Could not update participants");return}
    for _,person:=range people{
        _,err=tx.Exec(r.Context(),"INSERT INTO chore_assignees(chore_id,user_id) VALUES($1,$2) ON CONFLICT DO NOTHING",id,person)
        if err!=nil{writeError(w,500,"Could not update participants");return}
    }
    if err=tx.Commit(r.Context());err!=nil{writeError(w,500,"Could not save chore");return}
    s.audit(r,"update","chore",&id,"Updated household chore",nil)
    writeJSON(w,200,map[string]any{"id":id})
}

func (s *server) changeChoreStatus(w http.ResponseWriter,r *http.Request) {
    if !s.requireChores(w,r){return}
    id,err:=uuid.Parse(r.PathValue("id"));if err!=nil{writeError(w,400,"Invalid chore");return}
    var input choreStatusInput
    if decode(r,&input)!=nil || (input.Status!="pending"&&input.Status!="in_progress"&&input.Status!="done"){
        writeError(w,400,"Choose Pending, In progress, or Done");return
    }
    actor:=currentActor(r)
    target:=actor.ID
    if input.AssigneeID!="" {
        target,err=uuid.Parse(input.AssigneeID)
        if err!=nil||target==uuid.Nil{writeError(w,400,"Invalid assignee");return}
    }
    if target!=actor.ID&&!canManageChores(actor.Role){writeError(w,403,"You can only update your own chores");return}
    tx,err:=s.db.Begin(r.Context())
    if err!=nil{writeError(w,500,"Could not begin chore update");return}
    defer tx.Rollback(r.Context())
    var start time.Time
    var until *time.Time
    var rule string
    var primary uuid.UUID
    var kind string
    err=tx.QueryRow(r.Context(),
      "SELECT due_date,recurrence_rule,recurrence_until,assigned_to,kind FROM chores WHERE id=$1 FOR UPDATE",id).
      Scan(&start,&rule,&until,&primary,&kind)
    if errors.Is(err,pgx.ErrNoRows){writeError(w,404,"Chore not found");return}
    if err!=nil{writeError(w,500,"Could not find chore");return}
    if !s.requireWorkKind(w,r,kind){return}
    if actor.IsChild&&!childModuleAllowed(actor.ChildModules,childWorkModule(kind)){
        writeError(w,403,"A guardian has not enabled this type of work");return
    }
    if input.AssigneeID=="" && canManageChores(actor.Role){target=primary}
    date:=start.Format("2006-01-02")
    if input.OccurrenceDate!=""{date=input.OccurrenceDate}
    when,err:=choreDate(date)
    if err!=nil||!choreDueOn(start,when,rule)||(until!=nil&&when.After(*until)){
        writeError(w,400,"Choose a valid occurrence of this chore");return
    }
    if input.Status!="done" {
        var approved bool
        err=tx.QueryRow(r.Context(),`SELECT EXISTS(SELECT 1 FROM family_chore_reviews
          WHERE chore_id=$1 AND user_id=$2 AND occurrence_date=$3::date
          AND decision='approved')`,id,target,date).Scan(&approved)
        if err!=nil{writeError(w,500,"Could not check reward history");return}
        if approved{writeError(w,409,"This chore was already rewarded; an approved completion cannot be reopened");return}
    }
    tag,err:=tx.Exec(r.Context(),`INSERT INTO chore_progress
        (chore_id,user_id,occurrence_date,status,completed_at,completed_by)
        SELECT $1,$2,$3::date,$4,CASE WHEN $4='done' THEN now() ELSE NULL END,
               CASE WHEN $4='done' THEN $5::uuid ELSE NULL END
        WHERE EXISTS(SELECT 1 FROM chore_assignees WHERE chore_id=$1 AND user_id=$2)
        ON CONFLICT(chore_id,user_id,occurrence_date) DO UPDATE
        SET status=EXCLUDED.status,
            completed_at=CASE WHEN EXCLUDED.status='done'
              THEN COALESCE(chore_progress.completed_at,now()) ELSE NULL END,
            completed_by=EXCLUDED.completed_by,updated_at=now()`,
        id,target,date,input.Status,actor.ID)
    if err!=nil{writeError(w,500,"Could not update chore progress");return}
    if tag.RowsAffected()==0{writeError(w,403,"Only an assigned member or household planner can update this chore");return}
    if target==primary&&date==start.Format("2006-01-02"){
        _,err=tx.Exec(r.Context(),`UPDATE chores SET status=$2,
          completed_at=CASE WHEN $2='done' THEN now() ELSE NULL END,
          completed_by=CASE WHEN $2='done' THEN $3::uuid ELSE NULL END,
          updated_at=now() WHERE id=$1`,id,input.Status,actor.ID)
        if err!=nil{writeError(w,500,"Could not update chore summary");return}
    }
    if err=tx.Commit(r.Context());err!=nil{writeError(w,500,"Could not save chore progress");return}
    s.audit(r,"update","chore",&id,"Updated individual chore progress",
      map[string]any{"status":input.Status,"occurrence_date":date,"assignee_id":target})
    writeJSON(w,200,map[string]any{"id":id,"status":input.Status,"occurrence_date":date,"assignee_id":target})
}

func (s *server) deleteChore(w http.ResponseWriter,r *http.Request) {
    if !s.requireChores(w,r){return}
    if !canManageChores(currentActor(r).Role){writeError(w,403,"Only administrators and planners can delete chores");return}
    id,err:=uuid.Parse(r.PathValue("id"));if err!=nil{writeError(w,400,"Invalid chore");return}
    var kind string
    err=s.db.QueryRow(r.Context(),"SELECT kind FROM chores WHERE id=$1",id).Scan(&kind)
    if errors.Is(err,pgx.ErrNoRows){writeError(w,404,"Task or chore not found");return}
    if err!=nil{writeError(w,500,"Could not load task or chore");return}
    if !s.requireWorkKind(w,r,kind){return}
    var reviewed bool
    err=s.db.QueryRow(r.Context(),
      "SELECT EXISTS(SELECT 1 FROM family_chore_reviews WHERE chore_id=$1)",id).Scan(&reviewed)
    if err!=nil{writeError(w,500,"Could not check chore reward history");return}
    if reviewed{writeError(w,409,"This chore has reward history and cannot be deleted");return}
    tag,err:=s.db.Exec(r.Context(),"DELETE FROM chores WHERE id=$1",id)
    if err!=nil{writeError(w,500,"Could not delete chore");return}
    if tag.RowsAffected()==0{writeError(w,404,"Chore not found");return}
    s.audit(r,"delete","chore",&id,"Deleted household chore",nil)
    w.WriteHeader(http.StatusNoContent)
}
