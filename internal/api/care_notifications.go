package api

import (
 "context"
 "log"
 "strconv"
 "time"

 "github.com/google/uuid"
 "github.com/jackc/pgx/v5/pgxpool"
)

// generateCareNotifications uses the existing durable inbox and push queue.
// The occurrence's unique key prevents duplicates during retries/restarts.
func generateCareNotifications(ctx context.Context,db *pgxpool.Pool){
 tz:="UTC"
 _=db.QueryRow(ctx,"SELECT COALESCE((SELECT value FROM app_settings WHERE key='timezone'),'UTC')").Scan(&tz)
 loc,err:=time.LoadLocation(tz);if err!=nil{loc=time.UTC}
 today:=time.Now().In(loc).Format("2006-01-02")
 rows,err:=db.Query(ctx,`
 SELECT i.id,s.kind,s.name,i.title,i.starts_on,i.repeat_rule,i.reminder_days_before
 FROM family_care_items i JOIN family_care_subjects s ON s.id=i.subject_id
 JOIN app_settings st ON st.key=s.kind||'_enabled' AND st.value='true'
 WHERE i.archived=false AND s.archived=false AND i.starts_on<=($1::date + 30)
 ORDER BY i.id LIMIT 5000`,today)
 if err!=nil{log.Printf("Calden care reminders: scan failed: %v",err);return}
 type item struct{id uuid.UUID;kind,subject,title,repeat string;starts time.Time;lead int}
 items:=[]item{}
 for rows.Next(){
  var x item
  if err=rows.Scan(&x.id,&x.kind,&x.subject,&x.title,&x.starts,&x.repeat,&x.lead);err!=nil{break}
  items=append(items,x)
 }
 if err==nil{err=rows.Err()}
 rows.Close()
 if err!=nil{log.Printf("Calden care reminders: scan failed: %v",err);return}
 day,err:=time.Parse("2006-01-02",today)
 if err!=nil{return}
 for _,x:=range items{
  if ctx.Err()!=nil{return}
  if !careOccurrenceValid(x.starts,day,x.repeat){continue}
  label:=map[string]string{"school":"School","maintenance":"Home upkeep","pets":"Pet care"}[x.kind]
  // School reminders never include students' names or other private details.
  title:=cleanText(label+": "+x.title,200)
  if x.kind=="school" {title="School reminder"} // Never disclose student-identifiable text in delivery payloads.
  _,err=db.Exec(ctx,`
   INSERT INTO notifications(user_id,care_item_id,kind,title,message,occurrence_start)
   SELECT u.id,i.id,'care_due',$3,'Scheduled for today',($4::date::timestamp AT TIME ZONE 'UTC')
   FROM family_care_items i JOIN family_care_subjects s ON s.id=i.subject_id
   JOIN users u ON u.active=true AND u.is_child=false
   WHERE i.id=$1 AND s.kind=$2 AND i.archived=false AND s.archived=false
    AND NOT EXISTS(SELECT 1 FROM family_care_occurrences o
        WHERE o.item_id=i.id AND o.occurrence_date=$4::date)
    AND (
      (s.kind='school' AND (
          u.role='admin'
          OR (s.child_user_id IS NULL AND s.created_by=u.id)
          OR (s.child_user_id IS NOT NULL AND EXISTS(
             SELECT 1 FROM family_child_guardians g
             WHERE g.child_id=s.child_user_id AND g.guardian_id=u.id))
      ))
      OR (s.kind IN ('maintenance','pets') AND
          (u.role IN ('admin','planner') OR u.id=s.created_by OR u.id=i.assigned_to))
    )
   ON CONFLICT (user_id,care_item_id,kind,occurrence_start)
    WHERE kind='care_due' AND care_item_id IS NOT NULL AND occurrence_start IS NOT NULL
    DO NOTHING`,x.id,x.kind,title,today)
  if err!=nil{log.Printf("Calden care reminders: %s: %v",x.kind,err)}
 }
 // An advance notice is a separate kind, with a separate unique index.
 // The due-day reminder still fires for the same future occurrence.
 for _,x:=range items{
  if ctx.Err()!=nil{return}
  occurrence,scheduled:=careAdvanceOccurrence(x.starts,day,x.repeat,x.lead)
  if !scheduled{continue}
  dueDate:=occurrence.Format("2006-01-02")
  title:="Upcoming home upkeep"
  if x.kind=="pets" {title="Upcoming pet care"}
  if x.kind=="school" {title="Upcoming school reminder"}
  if x.kind!="school"{
    title=cleanText(title+": "+x.title,200)
  }
  message:="Due in "+strconv.Itoa(x.lead)+" day"
  if x.lead!=1 {message+="s"}
  _,err=db.Exec(ctx,`
   INSERT INTO notifications(user_id,care_item_id,kind,title,message,occurrence_start)
   SELECT u.id,i.id,'care_advance',$3,$4,($5::date::timestamp AT TIME ZONE 'UTC')
   FROM family_care_items i JOIN family_care_subjects s ON s.id=i.subject_id
   JOIN users u ON u.active=true AND u.is_child=false
   WHERE i.id=$1 AND s.kind=$2 AND i.archived=false AND s.archived=false
    AND i.reminder_days_before=$6
    AND NOT EXISTS(SELECT 1 FROM family_care_occurrences o
        WHERE o.item_id=i.id AND o.occurrence_date=$5::date)
    AND (
      (s.kind='school' AND (
          u.role='admin'
          OR (s.child_user_id IS NULL AND s.created_by=u.id)
          OR (s.child_user_id IS NOT NULL AND EXISTS(
             SELECT 1 FROM family_child_guardians g
             WHERE g.child_id=s.child_user_id AND g.guardian_id=u.id))
      ))
      OR (s.kind IN ('maintenance','pets') AND
          (u.role IN ('admin','planner') OR u.id=s.created_by OR u.id=i.assigned_to))
    )
   ON CONFLICT (user_id,care_item_id,kind,occurrence_start)
    WHERE kind='care_advance' AND care_item_id IS NOT NULL AND occurrence_start IS NOT NULL
    DO NOTHING`,x.id,x.kind,title,message,dueDate,x.lead)
  if err!=nil{log.Printf("Calden care advance: %s: %v",x.kind,err)}
 }
}
