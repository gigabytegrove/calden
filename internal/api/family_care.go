package api

import (
 "errors"
 "net/http"
 "strings"
 "time"

 "github.com/google/uuid"
 "github.com/jackc/pgx/v5"
)

var careModules=map[string]string{"school":"school_enabled","maintenance":"maintenance_enabled","pets":"pets_enabled"}
var careKinds=map[string]map[string]bool{
 "school":{"event":true,"deadline":true,"closure":true},
 "maintenance":{"service":true,"check":true},
 "pets":{"appointment":true,"vaccination":true,"medication":true},
}
type careSubjectInput struct {
 Name string `json:"name"`
 Details string `json:"details"`
 ChildUserID *uuid.UUID `json:"child_user_id"`
 SchoolName string `json:"school_name"`
 Grade string `json:"grade"`
 Archived bool `json:"archived"`
}
type careItemInput struct {
 Kind string `json:"kind"`
 Title string `json:"title"`
 Details string `json:"details"`
 Place string `json:"place"`
 Contact string `json:"contact"`
 StartsOn string `json:"starts_on"`
 RepeatRule string `json:"repeat_rule"`
 TermID *uuid.UUID `json:"term_id"`
 AssignedTo *uuid.UUID `json:"assigned_to"`
 Archived bool `json:"archived"`
 ReminderDaysBefore *int `json:"reminder_days_before"`
}
type careTermInput struct {
 Title string `json:"title"`
 BeginsOn string `json:"begins_on"`
 EndsOn string `json:"ends_on"`
}
func cleanCareSubject(in careSubjectInput,kind string)(careSubjectInput,bool){
 in.Name=strings.TrimSpace(in.Name)
 in.Details=strings.TrimSpace(in.Details)
 in.SchoolName=strings.TrimSpace(in.SchoolName)
 in.Grade=strings.TrimSpace(in.Grade)
 if len(in.Name)<1||len(in.Name)>140||len(in.Details)>4000||len(in.SchoolName)>140||len(in.Grade)>80{return in,false}
 if kind!="school" {
  if in.ChildUserID!=nil||in.SchoolName!=""||in.Grade!=""{return in,false}
 }
 return in,true
}
func cleanCareItem(in careItemInput,kind string)(careItemInput,bool){
 in.Title=strings.TrimSpace(in.Title)
 in.Details=strings.TrimSpace(in.Details)
 in.Place=strings.TrimSpace(in.Place)
 in.Contact=strings.TrimSpace(in.Contact)
 if !careKinds[kind][in.Kind]||len(in.Title)<1||len(in.Title)>180||len(in.Details)>4000||len(in.Place)>180||len(in.Contact)>180 {return in,false}
 if _,err:=choreDate(in.StartsOn);err!=nil{return in,false}
 if in.RepeatRule==""{in.RepeatRule="none"}
 switch in.RepeatRule{
 case "none","daily","weekly","monthly","yearly":
 default:return in,false
 }
 if kind!="school"&&in.TermID!=nil{return in,false}
 if in.ReminderDaysBefore!=nil&&!careReminderLeadAllowed(*in.ReminderDaysBefore){return in,false}
 if in.AssignedTo!=nil&&*in.AssignedTo==uuid.Nil{return in,false}
 return in,true
}
// Care reminders use explicit whole-day lead choices. Zero keeps the legacy
// due-day-only behavior for existing schedules and clients.
func careReminderLeadAllowed(days int) bool {
 switch days {
 case 0,1,2,3,7,14,30:return true
 default:return false
 }
}
func careAdvanceOccurrence(first,today time.Time,frequency string,lead int)(time.Time,bool){
 if !careReminderLeadAllowed(lead)||lead==0{return time.Time{},false}
 // AddDate uses civil dates: correct across DST and never multiplies
 // calendar days into fixed 24-hour periods in the household time zone.
 next:=today.AddDate(0,0,lead)
 return next,careOccurrenceValid(first,next,frequency)
}
func careOccurrenceValid(first,day time.Time,frequency string)bool{
 first=time.Date(first.Year(),first.Month(),first.Day(),0,0,0,0,time.UTC)
 day=time.Date(day.Year(),day.Month(),day.Day(),0,0,0,0,time.UTC)
 if day.Before(first){return false}
 switch frequency{
 case "none":return day.Equal(first)
 case "daily":return true
 case "weekly":return int(day.Sub(first).Hours()/24)%7==0
 case "monthly":
  if day.Year()==first.Year()&&day.Month()==first.Month(){return day.Equal(first)}
  last:=time.Date(day.Year(),day.Month()+1,0,0,0,0,0,time.UTC).Day()
  wanted:=first.Day();if wanted>last{wanted=last}
  return day.Day()==wanted
 case "yearly":
  if day.Month()!=first.Month(){return false}
  last:=time.Date(day.Year(),day.Month()+1,0,0,0,0,0,time.UTC).Day()
  wanted:=first.Day();if wanted>last{wanted=last}
  return day.Day()==wanted
 }
 return false
}
func careNextOccurrence(first,today time.Time,frequency string)string{
 if frequency=="none"&&first.Before(today){return first.Format("2006-01-02")}
 for i:=0;i<=370;i++ {
  d:=today.AddDate(0,0,i)
  if careOccurrenceValid(first,d,frequency){return d.Format("2006-01-02")}
 }
 return ""
}
func (s *server) careKind(w http.ResponseWriter,r *http.Request)(string,bool){
 kind:=r.PathValue("kind")
 setting,ok:=careModules[kind]
 if !ok{writeError(w,404,"This family section does not exist");return "",false}
 if currentActor(r).IsChild{writeError(w,403,"A guardian manages these family records");return "",false}
 var active bool
 err:=s.db.QueryRow(r.Context(),"SELECT COALESCE((SELECT value='true' FROM app_settings WHERE key=$1),false)",setting).Scan(&active)
 if err!=nil{writeError(w,500,"Could not check family settings");return "",false}
 if !active{writeError(w,403,"This family section is not enabled");return "",false}
 return kind,true
}
func (s *server) careCanManage(r *http.Request,created uuid.UUID)bool{
 return currentActor(r).Role=="admin"||currentActor(r).Role=="planner"||currentActor(r).ID==created
}
func (s *server) careGuardian(r *http.Request,child *uuid.UUID)bool{
 if child==nil{return true}
 if currentActor(r).Role=="admin"{return true}
 var yes bool
 err:=s.db.QueryRow(r.Context(),`SELECT EXISTS(
  SELECT 1 FROM family_child_guardians WHERE child_id=$1 AND guardian_id=$2)`,
  *child,currentActor(r).ID).Scan(&yes)
 return err==nil&&yes
}
func (s *server) careSubjectAccess(w http.ResponseWriter,r *http.Request,kind string,edit bool)(uuid.UUID,bool){
 id,err:=uuid.Parse(r.PathValue("id"))
 if err!=nil{writeError(w,400,"Invalid family record");return uuid.Nil,false}
 var owner uuid.UUID
 var child *uuid.UUID
 var archived bool
 err=s.db.QueryRow(r.Context(),`SELECT created_by,child_user_id,archived FROM family_care_subjects
  WHERE id=$1 AND kind=$2`,id,kind).Scan(&owner,&child,&archived)
 if errors.Is(err,pgx.ErrNoRows){writeError(w,404,"Family record not found");return uuid.Nil,false}
 if err!=nil{writeError(w,500,"Could not load family record");return uuid.Nil,false}
 if kind=="school" {
  if child==nil {
   if owner!=currentActor(r).ID&&currentActor(r).Role!="admin" {
    writeError(w,404,"Family record not found");return uuid.Nil,false
   }
  }else if !s.careGuardian(r,child){
   // A former guardian must immediately lose access, even to records they created.
   writeError(w,404,"Family record not found");return uuid.Nil,false
  }
 }
 if edit&&!s.careCanManage(r,owner){
  writeError(w,403,"Only the record owner or a household planner can change it");return uuid.Nil,false
 }
 return id,true
}
func (s *server) careValidStudent(r *http.Request,child *uuid.UUID)bool{
 if child==nil{return true}
 if !s.careGuardian(r,child){return false}
 var valid bool
 err:=s.db.QueryRow(r.Context(),"SELECT active AND is_child FROM users WHERE id=$1",*child).Scan(&valid)
 return err==nil&&valid
}
func (s *server) listCareSubjects(w http.ResponseWriter,r *http.Request){
 kind,ok:=s.careKind(w,r);if !ok{return}
 a:=currentActor(r)
 rows,err:=s.db.Query(r.Context(),`SELECT s.id,s.name,s.details,s.school_name,s.grade,
  s.child_user_id,s.archived,s.created_by,s.updated_at
  FROM family_care_subjects s WHERE s.kind=$1
   AND ($1<>'school' OR $3='admin'
     OR (s.child_user_id IS NULL AND s.created_by=$2)
     OR (s.child_user_id IS NOT NULL AND EXISTS(
       SELECT 1 FROM family_child_guardians g WHERE g.child_id=s.child_user_id AND g.guardian_id=$2)))
  ORDER BY s.archived,s.name,s.id`,kind,a.ID,a.Role)
 if err!=nil{writeError(w,500,"Could not load family records");return}
 defer rows.Close()
 out:=[]map[string]any{}
 for rows.Next(){
  var id,creator uuid.UUID;var child *uuid.UUID
  var name,details,school,grade string;var archived bool;var updated time.Time
  if err=rows.Scan(&id,&name,&details,&school,&grade,&child,&archived,&creator,&updated);err!=nil{break}
  out=append(out,map[string]any{"id":id,"name":name,"details":details,
    "school_name":school,"grade":grade,"child_user_id":child,
    "archived":archived,"created_by":creator,"updated_at":updated,
    "can_manage":s.careCanManage(r,creator)})
 }
 if err==nil{err=rows.Err()}
 if err!=nil{writeError(w,500,"Could not read family records");return}
 writeJSON(w,200,map[string]any{"items":out})
}
func (s *server) saveCareSubject(w http.ResponseWriter,r *http.Request){
 kind,ok:=s.careKind(w,r);if !ok{return}
 var in careSubjectInput
 if decode(r,&in)!=nil{writeError(w,400,"Check the family record");return}
 if in,ok=cleanCareSubject(in,kind);!ok{writeError(w,400,"Check the record name and details");return}
 if kind=="school"&&!s.careValidStudent(r,in.ChildUserID){
  writeError(w,403,"Choose a student you are authorized to manage");return
 }
 id:=uuid.Nil
 if r.Method==http.MethodPut {
  id,ok=s.careSubjectAccess(w,r,kind,true);if !ok{return}
  // School entries may not change their child linkage once recorded, protecting history.
  tag,err:=s.db.Exec(r.Context(),`UPDATE family_care_subjects SET
    name=$2,details=$3,school_name=$4,grade=$5,archived=$6,updated_at=now()
    WHERE id=$1 AND child_user_id IS NOT DISTINCT FROM $7::uuid`,
    id,in.Name,in.Details,in.SchoolName,in.Grade,in.Archived,in.ChildUserID)
  if err!=nil{writeError(w,500,"Could not save family record");return}
  if tag.RowsAffected()==0{writeError(w,409,"Student assignment cannot be changed after creation");return}
 }else{
  if !s.careCanManage(r,currentActor(r).ID){writeError(w,403,"Not permitted");return}
  err:=s.db.QueryRow(r.Context(),`INSERT INTO family_care_subjects
    (kind,name,details,child_user_id,school_name,grade,created_by)
    VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id`,
    kind,in.Name,in.Details,in.ChildUserID,in.SchoolName,in.Grade,currentActor(r).ID).Scan(&id)
  if err!=nil{writeError(w,500,"Could not add family record");return}
 }
 s.audit(r,"update","family_care_subject",&id,"Saved family care subject",map[string]any{"kind":kind})
 writeJSON(w,map[bool]int{true:200,false:201}[r.Method==http.MethodPut],map[string]any{"id":id})
}
func (s *server) listCareItems(w http.ResponseWriter,r *http.Request){
 kind,ok:=s.careKind(w,r);if !ok{return}
 id,ok:=s.careSubjectAccess(w,r,kind,false);if !ok{return}
 rows,err:=s.db.Query(r.Context(),`SELECT i.id,i.kind,i.title,i.details,i.place,i.contact,
   i.starts_on,i.repeat_rule,i.term_id,i.assigned_to,i.archived,i.created_by,i.reminder_days_before
   FROM family_care_items i WHERE i.subject_id=$1 ORDER BY i.archived,i.starts_on,i.id`,id)
 if err!=nil{writeError(w,500,"Could not load schedules");return}
 defer rows.Close()
 out:=[]map[string]any{}
 today:=s.routineToday(r)
 for rows.Next(){
  var itemID,creator uuid.UUID;var term,assigned *uuid.UUID
  var subkind,title,details,place,contact,frequency string
  var starts time.Time;var archived bool;var reminderDays int
  if err=rows.Scan(&itemID,&subkind,&title,&details,&place,&contact,&starts,&frequency,
   &term,&assigned,&archived,&creator,&reminderDays);err!=nil{break}
  next:="";if !archived{next=careNextOccurrence(starts,today,frequency)}
  out=append(out,map[string]any{"id":itemID,"kind":subkind,"title":title,
   "details":details,"place":place,"contact":contact,"starts_on":starts.Format("2006-01-02"),
   "repeat_rule":frequency,"term_id":term,"assigned_to":assigned,"archived":archived,
   "created_by":creator,"next_due":next,"reminder_days_before":reminderDays})
 }
 if err==nil{err=rows.Err()}
 if err!=nil{writeError(w,500,"Could not read family schedules");return}
 writeJSON(w,200,map[string]any{"items":out})
}
func (s *server) careValidateRefs(r *http.Request,kind string,subject uuid.UUID,in careItemInput)bool{
 if in.TermID!=nil {
  var exists bool
  err:=s.db.QueryRow(r.Context(),"SELECT EXISTS(SELECT 1 FROM family_school_terms WHERE id=$1 AND subject_id=$2)",*in.TermID,subject).Scan(&exists)
  if err!=nil||!exists{return false}
 }
 if in.AssignedTo!=nil{
  var active bool
  if err:=s.db.QueryRow(r.Context(),"SELECT active FROM users WHERE id=$1",*in.AssignedTo).Scan(&active);err!=nil||!active{return false}
 }
 return true
}
func (s *server) saveCareItem(w http.ResponseWriter,r *http.Request){
 kind,ok:=s.careKind(w,r);if !ok{return}
 subject,ok:=s.careSubjectAccess(w,r,kind,true);if !ok{return}
 var archived bool
 if err:=s.db.QueryRow(r.Context(),"SELECT archived FROM family_care_subjects WHERE id=$1",subject).Scan(&archived);err!=nil||archived{
  writeError(w,409,"Unarchive this record before editing its schedule");return
 }
 var in careItemInput
 if decode(r,&in)!=nil{writeError(w,400,"Check the scheduled item");return}
 if in,ok=cleanCareItem(in,kind);!ok{writeError(w,400,"Choose a valid kind, date and title");return}
 if !s.careValidateRefs(r,kind,subject,in){writeError(w,400,"Invalid student term or assigned person");return}
 id:=uuid.Nil;status:=201
 if r.Method==http.MethodPut {
  parsed,err:=uuid.Parse(r.PathValue("item_id"))
  if err!=nil{writeError(w,400,"Invalid schedule item");return}
  id=parsed;status=200
  tag,err:=s.db.Exec(r.Context(),`UPDATE family_care_items SET
   kind=$3,title=$4,details=$5,place=$6,contact=$7,starts_on=$8::date,
   repeat_rule=$9,term_id=$10,assigned_to=$11,archived=$12,
   reminder_days_before=COALESCE($13::integer,reminder_days_before),updated_at=now()
   WHERE id=$1 AND subject_id=$2`,
   id,subject,in.Kind,in.Title,in.Details,in.Place,in.Contact,in.StartsOn,
   in.RepeatRule,in.TermID,in.AssignedTo,in.Archived,in.ReminderDaysBefore)
  if err!=nil{writeError(w,500,"Could not update schedule");return}
  if tag.RowsAffected()==0{writeError(w,404,"Scheduled item not found");return}
 }else{
  err:=s.db.QueryRow(r.Context(),`INSERT INTO family_care_items
   (subject_id,kind,title,details,place,contact,starts_on,repeat_rule,term_id,assigned_to,created_by,reminder_days_before)
   VALUES($1,$2,$3,$4,$5,$6,$7::date,$8,$9,$10,$11,COALESCE($12::integer,0)) RETURNING id`,
   subject,in.Kind,in.Title,in.Details,in.Place,in.Contact,in.StartsOn,
   in.RepeatRule,in.TermID,in.AssignedTo,currentActor(r).ID,in.ReminderDaysBefore).Scan(&id)
  if err!=nil{writeError(w,500,"Could not add schedule");return}
 }
 s.audit(r,"update","family_care_item",&id,"Updated household care schedule",map[string]any{"kind":kind})
 writeJSON(w,status,map[string]any{"id":id})
}
func (s *server) careOccurrence(w http.ResponseWriter,r *http.Request){
 kind,ok:=s.careKind(w,r);if !ok{return}
 subject,ok:=s.careSubjectAccess(w,r,kind,true);if !ok{return}
 id,err:=uuid.Parse(r.PathValue("item_id"))
 if err!=nil{writeError(w,400,"Invalid item");return}
 var subjectArchived bool
 if err=s.db.QueryRow(r.Context(),"SELECT archived FROM family_care_subjects WHERE id=$1",subject).Scan(&subjectArchived);err!=nil{
  writeError(w,500,"Could not check record status");return
 }
 if subjectArchived{writeError(w,409,"Archived records cannot accept new completions");return}
 date,err:=choreDate(r.PathValue("date"))
 if err!=nil{writeError(w,400,"Invalid occurrence date");return}
 var in struct{Status string `json:"status"`;Note string `json:"note"`}
 if decode(r,&in)!=nil||(in.Status!="completed"&&in.Status!="skipped"){
  writeError(w,400,"Choose Completed or Skipped");return
 }
 in.Note=strings.TrimSpace(in.Note)
 if len(in.Note)>2000{writeError(w,400,"Completion note is too long");return}
 tx,err:=s.db.Begin(r.Context());if err!=nil{writeError(w,500,"Could not begin update");return}
 defer tx.Rollback(r.Context())
 var first time.Time;var rule string;var archived bool
 err=tx.QueryRow(r.Context(),`SELECT starts_on,repeat_rule,archived FROM family_care_items
   WHERE id=$1 AND subject_id=$2 FOR UPDATE`,id,subject).Scan(&first,&rule,&archived)
 if errors.Is(err,pgx.ErrNoRows){writeError(w,404,"Schedule not found");return}
 if err!=nil{writeError(w,500,"Could not read schedule");return}
 if archived||!careOccurrenceValid(first,date,rule){
  writeError(w,409,"That date is not an active occurrence");return
 }
 _,err=tx.Exec(r.Context(),`INSERT INTO family_care_occurrences(item_id,occurrence_date,status,note,recorded_by)
   VALUES($1,$2::date,$3,$4,$5)
   ON CONFLICT(item_id,occurrence_date) DO UPDATE SET status=EXCLUDED.status,
    note=EXCLUDED.note,recorded_by=EXCLUDED.recorded_by,recorded_at=now()`,
  id,date.Format("2006-01-02"),in.Status,in.Note,currentActor(r).ID)
 if err!=nil{writeError(w,500,"Could not record completion");return}
 // Keep notification rows for history, but hide outstanding prompts for this
 // occurrence once it was completed or deliberately skipped.
 _,err=tx.Exec(r.Context(),`UPDATE notifications SET dismissed_at=now()
   WHERE care_item_id=$1 AND kind IN ('care_due','care_advance')
     AND occurrence_start=($2::date::timestamp AT TIME ZONE 'UTC')
     AND dismissed_at IS NULL AND read_at IS NULL`,
   id,date.Format("2006-01-02"))
 if err!=nil{writeError(w,500,"Could not finish reminder update");return}
 if err=tx.Commit(r.Context());err!=nil{writeError(w,500,"Could not commit completion");return}
 s.audit(r,"update","family_care_occurrence",&id,"Recorded household care activity",
  map[string]any{"date":date.Format("2006-01-02"),"status":in.Status})
 writeJSON(w,200,map[string]any{"item_id":id,"date":date.Format("2006-01-02"),"status":in.Status})
}
func (s *server) careHistory(w http.ResponseWriter,r *http.Request){
 kind,ok:=s.careKind(w,r);if !ok{return}
 subject,ok:=s.careSubjectAccess(w,r,kind,false);if !ok{return}
 rows,err:=s.db.Query(r.Context(),`SELECT o.item_id,i.title,o.occurrence_date,o.status,
    o.note,o.recorded_at,u.display_name FROM family_care_occurrences o
    JOIN family_care_items i ON i.id=o.item_id
    JOIN users u ON u.id=o.recorded_by
    WHERE i.subject_id=$1
    ORDER BY o.occurrence_date DESC,o.recorded_at DESC LIMIT 150`,subject)
 if err!=nil{writeError(w,500,"Could not read care history");return}
 defer rows.Close()
 out:=[]map[string]any{}
 for rows.Next(){
  var id uuid.UUID;var title,status,note,person string;var day,recorded time.Time
  if err=rows.Scan(&id,&title,&day,&status,&note,&recorded,&person);err!=nil{break}
  out=append(out,map[string]any{"item_id":id,"title":title,"date":day.Format("2006-01-02"),
   "status":status,"note":note,"recorded_at":recorded,"recorded_by_name":person})
 }
 if err==nil{err=rows.Err()}
 if err!=nil{writeError(w,500,"Could not read care history");return}
 writeJSON(w,200,map[string]any{"items":out})
}
func (s *server) careTerms(w http.ResponseWriter,r *http.Request){
 kind,ok:=s.careKind(w,r);if !ok{return}
 if kind!="school"{writeError(w,404,"Terms apply only to school records");return}
 subject,ok:=s.careSubjectAccess(w,r,kind,r.Method==http.MethodPost);if !ok{return}
 if r.Method==http.MethodPost{
  var archived bool
  if err:=s.db.QueryRow(r.Context(),"SELECT archived FROM family_care_subjects WHERE id=$1",subject).Scan(&archived);err!=nil||archived{
   writeError(w,409,"Unarchive this student before adding a term");return
  }
  var in careTermInput
  if decode(r,&in)!=nil{writeError(w,400,"Check academic term");return}
  in.Title=strings.TrimSpace(in.Title)
  start,err1:=choreDate(in.BeginsOn);end,err2:=choreDate(in.EndsOn)
  if len(in.Title)<1||len(in.Title)>120||err1!=nil||err2!=nil||end.Before(start){
   writeError(w,400,"Choose a valid name and term dates");return
  }
  var id uuid.UUID
  err:=s.db.QueryRow(r.Context(),`INSERT INTO family_school_terms(subject_id,title,begins_on,ends_on,created_by)
   VALUES($1,$2,$3::date,$4::date,$5) RETURNING id`,
   subject,in.Title,in.BeginsOn,in.EndsOn,currentActor(r).ID).Scan(&id)
  if err!=nil{writeError(w,409,"Could not add academic term");return}
  s.audit(r,"create","school_term",&id,"Added academic term",nil)
  writeJSON(w,201,map[string]any{"id":id});return
 }
 rows,err:=s.db.Query(r.Context(),`SELECT id,title,begins_on,ends_on FROM family_school_terms
  WHERE subject_id=$1 ORDER BY begins_on DESC`,subject)
 if err!=nil{writeError(w,500,"Could not load academic terms");return}
 defer rows.Close()
 out:=[]map[string]any{}
 for rows.Next(){
  var id uuid.UUID;var title string;var start,end time.Time
  if err=rows.Scan(&id,&title,&start,&end);err!=nil{break}
  out=append(out,map[string]any{"id":id,"title":title,"begins_on":start.Format("2006-01-02"),"ends_on":end.Format("2006-01-02")})
 }
 if err==nil{err=rows.Err()}
 if err!=nil{writeError(w,500,"Could not read academic terms");return}
 writeJSON(w,200,map[string]any{"items":out})
}

