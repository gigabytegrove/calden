package api

import (
 "encoding/json"
 "errors"
 "net/http"
 "sort"
 "strings"
 "time"

 "github.com/google/uuid"
 "github.com/jackc/pgx/v5"
)

type routineStep struct {
 ID uuid.UUID `json:"id"`
 Title string `json:"title"`
}
type routineStepInput struct {
 ID string `json:"id,omitempty"`
 Title string `json:"title"`
}
type routineInput struct {
 Title string `json:"title"`
 Notes string `json:"notes"`
 AssignedTo uuid.UUID `json:"assigned_to"`
 TimeOfDay string `json:"time_of_day"`
 DaysOfWeek []int16 `json:"days_of_week"`
 Steps []routineStepInput `json:"steps"`
}
func normalizeRoutine(in routineInput)(routineInput,bool) {
 in.Title=strings.TrimSpace(in.Title)
 in.Notes=strings.TrimSpace(in.Notes)
 if in.TimeOfDay=="" {in.TimeOfDay="anytime"}
 if len(in.Title)<1||len(in.Title)>120||len(in.Notes)>2000||in.AssignedTo==uuid.Nil {
  return in,false
 }
 if in.TimeOfDay!="morning"&&in.TimeOfDay!="evening"&&in.TimeOfDay!="anytime" {return in,false}
 if len(in.DaysOfWeek)<1||len(in.DaysOfWeek)>7||len(in.Steps)<1||len(in.Steps)>30 {return in,false}
 days:=make(map[int16]bool,len(in.DaysOfWeek))
 for _,day:=range in.DaysOfWeek {
  if day<1||day>7||days[day]{return in,false}
  days[day]=true
 }
 sort.Slice(in.DaysOfWeek,func(i,j int)bool{return in.DaysOfWeek[i]<in.DaysOfWeek[j]})
 for i:=range in.Steps {
  in.Steps[i].Title=strings.TrimSpace(in.Steps[i].Title)
  if in.Steps[i].Title==""||len(in.Steps[i].Title)>180{return in,false}
  if in.Steps[i].ID!="" {
   id,err:=uuid.Parse(in.Steps[i].ID)
   if err!=nil||id==uuid.Nil{return in,false}
  }
 }
 return in,true
}
// Steps have server-generated, stable identifiers. Removed steps are no
// longer shown, but historical completion records survive unchanged.
func allocateRoutineSteps(inputs []routineStepInput,previous []routineStep)([]routineStep,error){
 allowed:=make(map[uuid.UUID]bool,len(previous))
 for _,step:=range previous{allowed[step.ID]=true}
 seen:=map[uuid.UUID]bool{}
 out:=make([]routineStep,0,len(inputs))
 for _,step:=range inputs{
  id:=uuid.New()
  if step.ID!="" {
   parsed,err:=uuid.Parse(step.ID)
   if err!=nil||parsed==uuid.Nil||!allowed[parsed]||seen[parsed] {
    return nil,errors.New("A routine step is invalid or no longer exists")
   }
   id=parsed
  }
  seen[id]=true
  out=append(out,routineStep{ID:id,Title:step.Title})
 }
 return out,nil
}
func routineDue(days []int16,date time.Time)bool{
 weekday:=int16(date.Weekday())
 if weekday==0{weekday=7}
 for _,day:=range days{if day==weekday{return true}}
 return false
}
func (s *server) requireRoutines(w http.ResponseWriter,r *http.Request)bool{
 var enabled bool
 err:=s.db.QueryRow(r.Context(),
 "SELECT COALESCE((SELECT value='true' FROM app_settings WHERE key='routines_enabled'),false)").Scan(&enabled)
 if err!=nil{writeError(w,500,"Could not check routine settings");return false}
 if !enabled{writeError(w,403,"Routines are turned off for this household");return false}
 return true
}
func (s *server) routineToday(r *http.Request)time.Time{
 tz:="UTC"
 _=s.db.QueryRow(r.Context(),
   "SELECT COALESCE((SELECT value FROM app_settings WHERE key='timezone'),'UTC')").Scan(&tz)
 loc,err:=time.LoadLocation(tz)
 if err!=nil{loc=time.UTC}
 day,_:=choreDate(time.Now().In(loc).Format("2006-01-02"))
 return day
}
func (s *server) routineDay(r *http.Request,value string)(time.Time,bool){
 if value==""{return s.routineToday(r),true}
 day,err:=choreDate(value)
 if err!=nil{return day,false}
 today:=s.routineToday(r)
 if day.Before(today.AddDate(0,0,-120))||day.After(today.AddDate(0,0,30)){return day,false}
 return day,true
}
func (s *server) routinesList(w http.ResponseWriter,r *http.Request){
 if !s.requireRoutines(w,r){return}
 date,ok:=s.routineDay(r,r.URL.Query().Get("date"))
 if !ok{writeError(w,400,"Choose a day within the allowed range");return}
 a:=currentActor(r)
 manage:=canManageChores(a.Role)
 rows,err:=s.db.Query(r.Context(),`
  SELECT r.id,r.title,r.notes,r.assigned_to,u.display_name,r.created_by,
   r.time_of_day,r.days_of_week,r.steps,r.created_at,r.updated_at
  FROM family_routines r JOIN users u ON u.id=r.assigned_to
  WHERE r.archived=false AND ($1::boolean OR r.assigned_to=$2)
  ORDER BY r.time_of_day,r.title,r.id`,manage,a.ID)
 if err!=nil{writeError(w,500,"Could not load routines");return}
 type item struct{
  id,assigned,creator uuid.UUID
  title,notes,name,slot string
  days []int16
  steps []routineStep
  created,updated time.Time
 }
 records:=[]item{}
 for rows.Next(){
  var v item
  var raw []byte
  if err=rows.Scan(&v.id,&v.title,&v.notes,&v.assigned,&v.name,&v.creator,
      &v.slot,&v.days,&raw,&v.created,&v.updated);err!=nil{break}
  if err=json.Unmarshal(raw,&v.steps);err!=nil{break}
  records=append(records,v)
 }
 if err==nil{err=rows.Err()}
 rows.Close()
 if err!=nil{writeError(w,500,"Could not read routines");return}
 completed:=map[string]bool{}
 rows,err=s.db.Query(r.Context(),`
  SELECT p.routine_id,p.user_id,p.step_id
  FROM family_routine_progress p JOIN family_routines r ON r.id=p.routine_id
  WHERE p.occurrence_date=$1::date
   AND ($2::boolean OR r.assigned_to=$3)`,date.Format("2006-01-02"),manage,a.ID)
 if err!=nil{writeError(w,500,"Could not load routine progress");return}
 for rows.Next(){
  var id,person,step uuid.UUID
  if err=rows.Scan(&id,&person,&step);err!=nil{break}
  completed[id.String()+"/"+person.String()+"/"+step.String()]=true
 }
 if err==nil{err=rows.Err()}
 rows.Close()
 if err!=nil{writeError(w,500,"Could not read routine progress");return}
 out:=[]map[string]any{}
 for _,record:=range records{
  stepItems:=[]map[string]any{}
  done:=0
  for _,step:=range record.steps{
   isDone:=completed[record.id.String()+"/"+record.assigned.String()+"/"+step.ID.String()]
   if isDone{done++}
   stepItems=append(stepItems,map[string]any{"id":step.ID,"title":step.Title,"completed":isDone})
  }
  out=append(out,map[string]any{
   "id":record.id,"title":record.title,"notes":record.notes,
   "assigned_to":record.assigned,"assignee_name":record.name,
   "created_by":record.creator,"time_of_day":record.slot,
   "days_of_week":record.days,"due_today":routineDue(record.days,date),
   "steps":stepItems,"done":done,"total":len(record.steps),
   "created_at":record.created,"updated_at":record.updated,
   "can_manage":manage,"can_complete":manage||record.assigned==a.ID,
  })
 }
 writeJSON(w,200,map[string]any{"date":date.Format("2006-01-02"),"items":out})
}
func (s *server) routineUserActive(r *http.Request,person uuid.UUID)bool{
 var active bool
 err:=s.db.QueryRow(r.Context(),"SELECT active FROM users WHERE id=$1",person).Scan(&active)
 return err==nil&&active
}
func (s *server) createRoutine(w http.ResponseWriter,r *http.Request){
 if !s.requireRoutines(w,r){return}
 if !canManageChores(currentActor(r).Role){writeError(w,403,"Only a household administrator or planner can create routines");return}
 var in routineInput
 if decode(r,&in)!=nil{writeError(w,400,"Check the routine details");return}
 clean,ok:=normalizeRoutine(in)
 if !ok{writeError(w,400,"Check the title, person, scheduled days, and checklist");return}
 if !s.routineUserActive(r,clean.AssignedTo){writeError(w,400,"Choose an active family member");return}
 steps,err:=allocateRoutineSteps(clean.Steps,nil)
 if err!=nil{writeError(w,400,err.Error());return}
 raw,err:=json.Marshal(steps)
 if err!=nil{writeError(w,500,"Could not prepare routine");return}
 var id uuid.UUID
 err=s.db.QueryRow(r.Context(),`INSERT INTO family_routines
  (title,notes,assigned_to,created_by,time_of_day,days_of_week,steps)
  VALUES($1,$2,$3,$4,$5,$6,$7::jsonb) RETURNING id`,
  clean.Title,clean.Notes,clean.AssignedTo,currentActor(r).ID,
  clean.TimeOfDay,clean.DaysOfWeek,string(raw)).Scan(&id)
 if err!=nil{writeError(w,500,"Could not create routine");return}
 s.audit(r,"create","routine",&id,"Created family routine",nil)
 writeJSON(w,201,map[string]any{"id":id})
}
func (s *server) updateRoutine(w http.ResponseWriter,r *http.Request){
 if !s.requireRoutines(w,r){return}
 if !canManageChores(currentActor(r).Role){writeError(w,403,"Only an administrator or planner can edit routines");return}
 id,err:=uuid.Parse(r.PathValue("id"))
 if err!=nil{writeError(w,400,"Invalid routine");return}
 var in routineInput
 if decode(r,&in)!=nil{writeError(w,400,"Check the routine details");return}
 clean,ok:=normalizeRoutine(in)
 if !ok{writeError(w,400,"Check the title, person, scheduled days, and checklist");return}
 if !s.routineUserActive(r,clean.AssignedTo){writeError(w,400,"Choose an active family member");return}
 tx,err:=s.db.Begin(r.Context())
 if err!=nil{writeError(w,500,"Could not update routine");return}
 defer tx.Rollback(r.Context())
 var raw []byte
 err=tx.QueryRow(r.Context(),
   "SELECT steps FROM family_routines WHERE id=$1 AND archived=false FOR UPDATE",id).Scan(&raw)
 if errors.Is(err,pgx.ErrNoRows){writeError(w,404,"Routine not found");return}
 if err!=nil{writeError(w,500,"Could not read routine");return}
 var existing []routineStep
 if json.Unmarshal(raw,&existing)!=nil{writeError(w,500,"Stored routine is invalid");return}
 steps,err:=allocateRoutineSteps(clean.Steps,existing)
 if err!=nil{writeError(w,400,err.Error());return}
 payload,err:=json.Marshal(steps)
 if err!=nil{writeError(w,500,"Could not prepare routine");return}
 _,err=tx.Exec(r.Context(),`UPDATE family_routines SET title=$2,notes=$3,
  assigned_to=$4,time_of_day=$5,days_of_week=$6,steps=$7::jsonb,updated_at=now()
  WHERE id=$1 AND archived=false`,id,clean.Title,clean.Notes,clean.AssignedTo,
  clean.TimeOfDay,clean.DaysOfWeek,string(payload))
 if err!=nil{writeError(w,500,"Could not update routine");return}
 if err=tx.Commit(r.Context());err!=nil{writeError(w,500,"Could not save routine");return}
 s.audit(r,"update","routine",&id,"Updated family routine",nil)
 writeJSON(w,200,map[string]any{"id":id})
}
func (s *server) archiveRoutine(w http.ResponseWriter,r *http.Request){
 if !s.requireRoutines(w,r){return}
 if !canManageChores(currentActor(r).Role){writeError(w,403,"Only a household administrator or planner can archive routines");return}
 id,err:=uuid.Parse(r.PathValue("id"))
 if err!=nil{writeError(w,400,"Invalid routine");return}
 tag,err:=s.db.Exec(r.Context(),
 "UPDATE family_routines SET archived=true,updated_at=now() WHERE id=$1 AND archived=false",id)
 if err!=nil{writeError(w,500,"Could not archive routine");return}
 if tag.RowsAffected()==0{writeError(w,404,"Routine not found");return}
 s.audit(r,"archive","routine",&id,"Archived family routine without removing completion history",nil)
 w.WriteHeader(http.StatusNoContent)
}
func (s *server) updateRoutineStep(w http.ResponseWriter,r *http.Request){
 if !s.requireRoutines(w,r){return}
 id,err:=uuid.Parse(r.PathValue("id"))
 if err!=nil{writeError(w,400,"Invalid routine");return}
 stepID,err:=uuid.Parse(r.PathValue("step_id"))
 if err!=nil{writeError(w,400,"Invalid checklist item");return}
 var in struct{
  Date string `json:"date"`
  Completed *bool `json:"completed"`
 }
 if decode(r,&in)!=nil||in.Completed==nil{writeError(w,400,"Choose whether the step is complete");return}
 day,ok:=s.routineDay(r,in.Date)
 if !ok{writeError(w,400,"Choose a day within the allowed range");return}
 tx,err:=s.db.Begin(r.Context())
 if err!=nil{writeError(w,500,"Could not update checklist");return}
 defer tx.Rollback(r.Context())
 var assigned uuid.UUID
 var days []int16
 var raw []byte
 err=tx.QueryRow(r.Context(),
 "SELECT assigned_to,days_of_week,steps FROM family_routines WHERE id=$1 AND archived=false FOR UPDATE",
 id).Scan(&assigned,&days,&raw)
 if errors.Is(err,pgx.ErrNoRows){writeError(w,404,"Routine not found");return}
 if err!=nil{writeError(w,500,"Could not load routine");return}
 actor:=currentActor(r)
 if assigned!=actor.ID&&!canManageChores(actor.Role){
  writeError(w,403,"You can only complete your assigned routines");return
 }
 if !routineDue(days,day){writeError(w,400,"This routine is not scheduled on that day");return}
 var steps []routineStep
 if json.Unmarshal(raw,&steps)!=nil{writeError(w,500,"Could not read checklist");return}
 var stepTitle string
 for _,step:=range steps{if step.ID==stepID{stepTitle=step.Title;break}}
 if stepTitle==""{writeError(w,404,"Checklist item not found");return}
 if *in.Completed {
  _,err=tx.Exec(r.Context(),`INSERT INTO family_routine_progress
    (routine_id,user_id,step_id,occurrence_date,step_title_snapshot,completed_by)
    VALUES($1,$2,$3,$4::date,$5,$6)
    ON CONFLICT(routine_id,user_id,step_id,occurrence_date) DO NOTHING`,
    id,assigned,stepID,day.Format("2006-01-02"),stepTitle,actor.ID)
 }else{
  _,err=tx.Exec(r.Context(),`DELETE FROM family_routine_progress
    WHERE routine_id=$1 AND user_id=$2 AND step_id=$3 AND occurrence_date=$4::date`,
    id,assigned,stepID,day.Format("2006-01-02"))
 }
 if err!=nil{writeError(w,500,"Could not update checklist");return}
 if err=tx.Commit(r.Context());err!=nil{writeError(w,500,"Could not save checklist");return}
 s.audit(r,"update","routine_step",&id,"Updated routine checklist",map[string]any{
  "step_id":stepID,"assigned_to":assigned,"date":day.Format("2006-01-02"),"completed":*in.Completed,
 })
 writeJSON(w,200,map[string]any{"routine_id":id,"step_id":stepID,
  "date":day.Format("2006-01-02"),"completed":*in.Completed})
}
