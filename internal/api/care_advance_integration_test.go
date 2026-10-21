package api

import (
 "context"
 "os"
 "strings"
 "testing"
 "time"

 "github.com/gigabytegrove/calden/internal/store"
 "github.com/google/uuid"
 "github.com/jackc/pgx/v5"
 "github.com/jackc/pgx/v5/pgxpool"
)

// Uses a dedicated PostgreSQL database only. It exercises the real migration,
// scheduling worker, private school recipient policy and conflict indexes.
func TestCareAdvanceNotificationsDatabase(t *testing.T){
 raw:=strings.TrimSpace(os.Getenv("CALDEN_TEST_DATABASE_URL"))
 if raw==""{t.Skip("Set CALDEN_TEST_DATABASE_URL for isolated database coverage")}
 cfg,err:=pgxpool.ParseConfig(raw)
 if err!=nil{t.Fatal(err)}
 dbName:=cfg.ConnConfig.Database
 if dbName!="calden_test"&&!strings.HasPrefix(dbName,"calden_test_"){
  t.Fatalf("Refusing non-test database: %q",dbName)
 }
 ctx,cancel:=context.WithTimeout(context.Background(),90*time.Second)
 defer cancel()
 db,err:=store.Open(ctx,raw)
 if err!=nil{t.Fatal(err)}
 // Clean fixtures before closing the database; t.Cleanup callbacks run after
 // normal defers, so an earlier defer db.Close would be incorrect.
 var subjectID uuid.UUID
 var users []uuid.UUID
 oldSchool:="false"
 if err:=store.Migrate(ctx,db);err!=nil{db.Close();t.Fatal(err)}
 err=db.QueryRow(ctx,`SELECT value FROM app_settings WHERE key='school_enabled'`).Scan(&oldSchool)
 if err!=nil&&err!=pgx.ErrNoRows{db.Close();t.Fatal(err)}
 t.Cleanup(func(){
  cleanup,cancel:=context.WithTimeout(context.Background(),15*time.Second)
  defer cancel()
  if subjectID!=uuid.Nil{
   _,_=db.Exec(cleanup,`DELETE FROM family_care_occurrences WHERE item_id IN
    (SELECT id FROM family_care_items WHERE subject_id=$1)`,subjectID)
   _,_=db.Exec(cleanup,`DELETE FROM family_care_items WHERE subject_id=$1`,subjectID)
   _,_=db.Exec(cleanup,`DELETE FROM family_care_subjects WHERE id=$1`,subjectID)
  }
  for _,id:=range users{
   _,_=db.Exec(cleanup,`DELETE FROM family_child_guardians WHERE child_id=$1 OR guardian_id=$1`,id)
   _,_=db.Exec(cleanup,`DELETE FROM family_child_profiles WHERE user_id=$1`,id)
   _,_=db.Exec(cleanup,`DELETE FROM users WHERE id=$1`,id)
  }
  _,_=db.Exec(cleanup,`UPDATE app_settings SET value=$1 WHERE key='school_enabled'`,oldSchool)
  db.Close()
 })
 // A test-only module enablement is restored during cleanup.
 if _,err=db.Exec(ctx,`INSERT INTO app_settings(key,value) VALUES('school_enabled','true')
   ON CONFLICT(key) DO UPDATE SET value=EXCLUDED.value`);err!=nil{t.Fatal(err)}
 roleList:=[]string{"admin","member","member","restricted"}
 for i,role:=range roleList{
  username:="care_advance_test_"+uuid.NewString()
  var id uuid.UUID
  err=db.QueryRow(ctx,`INSERT INTO users(username,display_name,password_hash,role,initials,is_child)
    VALUES($1,$2,'unusable-test-password',$3,'TT',$4) RETURNING id`,
    username,"Test",role,i==3).Scan(&id)
  if err!=nil{t.Fatal(err)}
  users=append(users,id)
 }
 // Admin and guardian should receive alerts. Unrelated adult and the
 // restricted child should never receive the school record's alerts.
 if _,err=db.Exec(ctx,`INSERT INTO family_child_profiles(user_id) VALUES($1)`,users[3]);err!=nil{t.Fatal(err)}
 if _,err=db.Exec(ctx,`INSERT INTO family_child_guardians(child_id,guardian_id)
   VALUES($1,$2)`,users[3],users[1]);err!=nil{t.Fatal(err)}
 if err=db.QueryRow(ctx,`INSERT INTO family_care_subjects
   (kind,name,child_user_id,created_by)
   VALUES('school','Private school record',$1,$2) RETURNING id`,
   users[3],users[1]).Scan(&subjectID);err!=nil{t.Fatal(err)}

 tz:="UTC"
 _=db.QueryRow(ctx,`SELECT COALESCE((SELECT value FROM app_settings
    WHERE key='timezone'),'UTC')`).Scan(&tz)
 loc,err:=time.LoadLocation(tz);if err!=nil{loc=time.UTC}
 today:=time.Now().In(loc)
 nowDay:=today.Format("2006-01-02")
 futureDay:=today.AddDate(0,0,7).Format("2006-01-02")
 for _,v:=range []struct{title,day string;lead int}{
  {"School today",nowDay,0},{"School next week",futureDay,7},
 }{
  _,err=db.Exec(ctx,`INSERT INTO family_care_items
    (subject_id,kind,title,starts_on,repeat_rule,reminder_days_before,created_by)
    VALUES($1,'event',$2,$3::date,'none',$4,$5)`,
    subjectID,v.title,v.day,v.lead,users[1])
  if err!=nil{t.Fatal(err)}
 }
 generateCareNotifications(ctx,db)
 generateCareNotifications(ctx,db)

 rows,err:=db.Query(ctx,`SELECT n.kind,n.title,n.user_id,count(*)
  FROM notifications n JOIN family_care_items i ON i.id=n.care_item_id
  WHERE i.subject_id=$1 GROUP BY n.kind,n.title,n.user_id`,subjectID)
 if err!=nil{t.Fatal(err)}
 defer rows.Close()
 counts:=map[string]int{}
 for rows.Next(){
  var kind,title string
  var recipient uuid.UUID
  var num int
  if err=rows.Scan(&kind,&title,&recipient,&num);err!=nil{break}
  if recipient!=users[0]&&recipient!=users[1]{
   t.Errorf("Private school alert reached unauthorized account")
  }
  if strings.Contains(title,"School today")||strings.Contains(title,"School next week"){
   t.Error("Private school title leaked in notification payload")
  }
  if num!=1{t.Errorf("Duplicate %s notification for same occurrence",kind)}
  counts[kind]+=num
 }
 if err==nil{err=rows.Err()}
 if err!=nil{t.Fatal(err)}
 if counts["care_due"]!=2||counts["care_advance"]!=2{
  t.Fatalf("Expected two due and two advance notices, got %#v",counts)
 }
}
