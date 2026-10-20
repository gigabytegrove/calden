package api

import (
 "errors"
 "fmt"
 "net/http"
 "strings"
 "time"

 "github.com/google/uuid"
 "github.com/jackc/pgx/v5"
)

type familySection struct {Key,Name string}
var familySections=map[string]familySection{
 "shopping":{"shopping","Shopping lists"},
 "school":{"school","School activities"},
 "maintenance":{"maintenance","Home maintenance"},
 "pets":{"pets","Pet care"},
 "routines":{"routines","Family routines"},
}
var familyRepeat=map[string]bool{"none":true,"daily":true,"weekly":true,"biweekly":true,"monthly":true,"yearly":true}

type familyItemInput struct {
 Title string `json:"title"`
 Notes string `json:"notes"`
 Subject string `json:"subject"`
 ListName string `json:"list_name"`
 Quantity float64 `json:"quantity"`
 Unit string `json:"unit"`
 DueOn string `json:"due_on"`
 Recurrence string `json:"recurrence"`
 Private bool `json:"private"`
}

func validateFamilyItem(in familyItemInput) (familyItemInput,error) {
 in.Title=strings.TrimSpace(in.Title)
 in.Notes=strings.TrimSpace(in.Notes)
 in.Subject=strings.TrimSpace(in.Subject)
 in.ListName=strings.TrimSpace(in.ListName)
 in.Unit=strings.TrimSpace(in.Unit)
 if in.Title==""||len(in.Title)>160||len(in.Notes)>4000||len(in.Subject)>100||len(in.ListName)>100||len(in.Unit)>24 {
  return in,errors.New("Check the name, notes, group, and unit lengths")
 }
 if in.Quantity==0 {in.Quantity=1}
 if in.Quantity<=0||in.Quantity>1000000 {return in,errors.New("Quantity must be between 0 and 1,000,000")}
 if in.Recurrence=="" {in.Recurrence="none"}
 if !familyRepeat[in.Recurrence] {return in,errors.New("Choose a valid repeat schedule")}
 if in.DueOn!="" {
  date,err:=choreDate(in.DueOn)
  if err!=nil||date.Year()<1900||date.Year()>2100 {return in,errors.New("Choose a valid date")}
 } else if in.Recurrence!="none" {
  return in,errors.New("Repeating items need a start date")
 }
 return in,nil
}
func (s *server) familyModuleEnabled(r *http.Request,section string)(bool,error){
 if _,ok:=familySections[section];!ok{return false,errors.New("unknown household section")}
 var enabled bool
 err:=s.db.QueryRow(r.Context(),
  "SELECT COALESCE((SELECT value='true' FROM app_settings WHERE key=$1),false)",
  "family_"+section+"_enabled").Scan(&enabled)
 return enabled,err
}
func (s *server) familyGate(w http.ResponseWriter,r *http.Request)(string,bool){
 section:=r.PathValue("section")
 if _,ok:=familySections[section];!ok {writeError(w,404,"Household feature not found");return "",false}
 enabled,err:=s.familyModuleEnabled(r,section)
 if err!=nil {writeError(w,500,"Could not load household features");return "",false}
 if !enabled {writeError(w,403,"This feature is turned off in Family settings");return "",false}
 return section,true
}
func (s *server) listFamilyFeatures(w http.ResponseWriter,r *http.Request){
 result:=map[string]bool{}
 for section:=range familySections{
  enabled,err:=s.familyModuleEnabled(r,section)
  if err!=nil{writeError(w,500,"Could not load household features");return}
  result[section]=enabled
 }
 writeJSON(w,200,map[string]any{"features":result})
}
func (s *server) saveFamilyFeatures(w http.ResponseWriter,r *http.Request){
 var in struct {Features map[string]bool `json:"features"`}
 if decode(r,&in)!=nil||len(in.Features)==0||len(in.Features)>len(familySections){
  writeError(w,400,"Choose a household feature");return
 }
 for section:=range in.Features{
  if _,ok:=familySections[section];!ok{writeError(w,400,"Unknown household feature");return}
 }
 tx,err:=s.db.Begin(r.Context())
 if err!=nil{writeError(w,500,"Could not save features");return}
 defer tx.Rollback(r.Context())
 for section,enabled:=range in.Features{
  value:="false";if enabled{value="true"}
  if _,err=tx.Exec(r.Context(),`
   INSERT INTO app_settings(key,value) VALUES($1,$2)
   ON CONFLICT(key) DO UPDATE SET value=EXCLUDED.value,updated_at=now()`,
   "family_"+section+"_enabled",value);err!=nil{
   writeError(w,500,"Could not save features");return
  }
 }
 if err=tx.Commit(r.Context());err!=nil{writeError(w,500,"Could not save features");return}
 s.audit(r,"update","settings",nil,"Updated household feature availability",map[string]any{"features":in.Features})
 s.listFamilyFeatures(w,r)
}
func familyAccessSQL()string{
 return " AND (owner_id IS NULL OR owner_id=$2)"
}
func (s *server) listFamilyItems(w http.ResponseWriter,r *http.Request){
 section,ok:=s.familyGate(w,r);if !ok{return}
 date:=r.URL.Query().Get("on")
 if date==""{
  loc:=time.UTC
  var tz string
  if s.db.QueryRow(r.Context(),"SELECT COALESCE((SELECT value FROM app_settings WHERE key='timezone'),'UTC')").Scan(&tz)==nil{
   if zone,err:=time.LoadLocation(tz);err==nil{loc=zone}
  }
  date=time.Now().In(loc).Format("2006-01-02")
 }
 if _,err:=choreDate(date);err!=nil{writeError(w,400,"Choose a valid date");return}
 rows,err:=s.db.Query(r.Context(),`
  SELECT i.id,i.title,i.notes,i.subject,i.list_name,i.quantity::TEXT,i.unit,
  COALESCE(to_char(i.due_on,'YYYY-MM-DD'),''),i.recurrence,i.private,
  i.created_by,i.created_at,i.updated_at,
  EXISTS(SELECT 1 FROM household_item_completions c WHERE c.item_id=i.id
   AND c.occurrence_date=CASE WHEN i.recurrence='none' THEN DATE '2000-01-01' ELSE $3::date END)
  FROM household_items i
  WHERE i.section=$1 AND (i.owner_id IS NULL OR i.owner_id=$2)
  ORDER BY i.due_on ASC NULLS LAST,i.updated_at DESC LIMIT 500`,
  section,currentActor(r).ID,date)
 if err!=nil{writeError(w,500,"Could not load household items");return}
 defer rows.Close()
 items:=[]map[string]any{}
 for rows.Next(){
  var id,creator uuid.UUID;var title,notes,subject,group,qty,unit,due,repeat string
  var private,done bool;var created,updated time.Time
  if err=rows.Scan(&id,&title,&notes,&subject,&group,&qty,&unit,&due,&repeat,&private,
   &creator,&created,&updated,&done);err!=nil{break}
  items=append(items,map[string]any{"id":id,"title":title,"notes":notes,
   "subject":subject,"list_name":group,"quantity":qty,"unit":unit,"due_on":due,
   "recurrence":repeat,"private":private,"created_by":creator,
   "created_at":created,"updated_at":updated,"done":done,
   "can_edit":creator==currentActor(r).ID || (!private && currentActor(r).Role=="admin")})
 }
 if err==nil{err=rows.Err()}
 if err!=nil{writeError(w,500,"Could not read household items");return}
 writeJSON(w,200,map[string]any{"items":items,"on":date})
}
func (s *server) createFamilyItem(w http.ResponseWriter,r *http.Request){
 section,ok:=s.familyGate(w,r);if !ok{return}
 var in familyItemInput
 if decode(r,&in)!=nil{writeError(w,400,"Check the item details");return}
 in,err:=validateFamilyItem(in);if err!=nil{writeError(w,400,err.Error());return}
 var due any
 if in.DueOn!=""{due=in.DueOn}
 var owner any
 if in.Private{owner=currentActor(r).ID}
 var id uuid.UUID
 err=s.db.QueryRow(r.Context(),`INSERT INTO household_items
 (section,title,notes,subject,list_name,quantity,unit,due_on,recurrence,private,owner_id,created_by)
 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) RETURNING id`,
 section,in.Title,in.Notes,in.Subject,in.ListName,in.Quantity,in.Unit,due,in.Recurrence,
 in.Private,owner,currentActor(r).ID).Scan(&id)
 if err!=nil{writeError(w,500,"Could not save the item");return}
 s.audit(r,"create","household_item",&id,"Added "+section+" item",nil)
 writeJSON(w,201,map[string]any{"id":id})
}
func familyItemID(r *http.Request)(uuid.UUID,error){
 id,err:=uuid.Parse(r.PathValue("id"));if err!=nil||id==uuid.Nil{
  return uuid.Nil,fmt.Errorf("invalid item")}
 return id,nil
}
func (s *server) updateFamilyItem(w http.ResponseWriter,r *http.Request){
 section,ok:=s.familyGate(w,r);if !ok{return}
 id,err:=familyItemID(r);if err!=nil{writeError(w,400,"Invalid item");return}
 var in familyItemInput
 if decode(r,&in)!=nil{writeError(w,400,"Check the item details");return}
 in,err=validateFamilyItem(in);if err!=nil{writeError(w,400,err.Error());return}
 var due,owner any
 if in.DueOn!=""{due=in.DueOn}
 if in.Private{owner=currentActor(r).ID}
 tag,err:=s.db.Exec(r.Context(),`UPDATE household_items SET
 title=$4,notes=$5,subject=$6,list_name=$7,quantity=$8,unit=$9,due_on=$10,
 recurrence=$11,private=$12,owner_id=$13,updated_at=now()
 WHERE id=$1 AND section=$2 AND (created_by=$3 OR (owner_id IS NULL AND $14='admin'))
 AND (owner_id IS NULL OR owner_id=$3)`,
 id,section,currentActor(r).ID,in.Title,in.Notes,in.Subject,in.ListName,in.Quantity,
 in.Unit,due,in.Recurrence,in.Private,owner,currentActor(r).Role)
 if err!=nil{writeError(w,500,"Could not save the item");return}
 if tag.RowsAffected()==0{writeError(w,404,"Item not found or you cannot edit it");return}
 s.audit(r,"update","household_item",&id,"Updated "+section+" item",nil)
 writeJSON(w,200,map[string]any{"id":id})
}
func (s *server) deleteFamilyItem(w http.ResponseWriter,r *http.Request){
 section,ok:=s.familyGate(w,r);if !ok{return}
 id,err:=familyItemID(r);if err!=nil{writeError(w,400,"Invalid item");return}
 tag,err:=s.db.Exec(r.Context(),`DELETE FROM household_items
 WHERE id=$1 AND section=$2 AND (created_by=$3 OR (owner_id IS NULL AND $4='admin'))
 AND (owner_id IS NULL OR owner_id=$3)`,
 id,section,currentActor(r).ID,currentActor(r).Role)
 if err!=nil{writeError(w,500,"Could not delete the item");return}
 if tag.RowsAffected()==0{writeError(w,404,"Item not found or you cannot delete it");return}
 s.audit(r,"delete","household_item",&id,"Deleted "+section+" item",nil)
 w.WriteHeader(204)
}
func (s *server) completeFamilyItem(w http.ResponseWriter,r *http.Request){
 section,ok:=s.familyGate(w,r);if !ok{return}
 id,err:=familyItemID(r);if err!=nil{writeError(w,400,"Invalid item");return}
 var in struct{On string `json:"on"`;Done *bool `json:"done"`}
 if decode(r,&in)!=nil||in.Done==nil {writeError(w,400,"Choose a completion state");return}
 var repeat string;var due *time.Time
 err=s.db.QueryRow(r.Context(),`SELECT recurrence,due_on FROM household_items
 WHERE id=$1 AND section=$2 AND (owner_id IS NULL OR owner_id=$3)`,
 id,section,currentActor(r).ID).Scan(&repeat,&due)
 if errors.Is(err,pgx.ErrNoRows){writeError(w,404,"Item not found");return}
 if err!=nil{writeError(w,500,"Could not load the item");return}
 occurrence:="2000-01-01"
 if repeat!="none"{
  selected,e:=choreDate(in.On)
  if e!=nil||due==nil||!choreDueOn(*due,selected,repeat){
   writeError(w,400,"Choose a scheduled occurrence date");return
  }
  occurrence=in.On
 }
 if *in.Done{
  _,err=s.db.Exec(r.Context(),`INSERT INTO household_item_completions(item_id,occurrence_date,completed_by)
   VALUES($1,$2,$3) ON CONFLICT(item_id,occurrence_date) DO NOTHING`,id,occurrence,currentActor(r).ID)
 }else{
  // Only the user who completed an entry, or an administrator on a shared entry, may reverse it.
  _,err=s.db.Exec(r.Context(),`DELETE FROM household_item_completions c USING household_items i
   WHERE c.item_id=i.id AND i.id=$1 AND i.section=$2
   AND (i.owner_id IS NULL OR i.owner_id=$3) AND c.occurrence_date=$4
   AND (c.completed_by=$3 OR ($5='admin' AND i.owner_id IS NULL))`,
   id,section,currentActor(r).ID,occurrence,currentActor(r).Role)
 }
 if err!=nil{writeError(w,500,"Could not update completion");return}
 s.audit(r,"update","household_item",&id,"Changed "+section+" completion",map[string]any{"on":occurrence,"done":*in.Done})
 writeJSON(w,200,map[string]any{"id":id,"on":occurrence})
}
