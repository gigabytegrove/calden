package api

import (
 "errors"
 "net/http"
 "strings"
 "time"

 "github.com/google/uuid"
 "github.com/jackc/pgx/v5"
)

func (s *server) requireRewards(w http.ResponseWriter,r *http.Request) bool {
 var enabled bool
 err:=s.db.QueryRow(r.Context(),
  "SELECT COALESCE((SELECT value='true' FROM app_settings WHERE key='rewards_enabled'),false)").Scan(&enabled)
 if err!=nil {writeError(w,500,"Could not read rewards settings");return false}
 if !enabled {writeError(w,403,"Rewards and allowances are turned off");return false}
 return true
}
func rewardManager(r *http.Request) bool {return canManageChores(currentActor(r).Role)}
func requiredRewardManager(w http.ResponseWriter,r *http.Request) bool {
 if rewardManager(r){return true}
 writeError(w,403,"A household administrator or planner must approve this action")
 return false
}
type rewardCatalogInput struct {
 Title string `json:"title"`
 Description string `json:"description"`
 CostPoints int `json:"cost_points"`
 Active bool `json:"active"`
}
func validRewardCatalog(in rewardCatalogInput)(rewardCatalogInput,bool){
 in.Title=strings.TrimSpace(in.Title)
 in.Description=strings.TrimSpace(in.Description)
 return in,len(in.Title)>0&&len(in.Title)<=140&&len(in.Description)<=1500&&in.CostPoints>=1&&in.CostPoints<=100000
}
func rewardDate(value string)(time.Time,error){return choreDate(value)}
func allowanceAdvance(date time.Time,frequency string,anchor int)time.Time{
 if frequency=="weekly"{return date.AddDate(0,0,7)}
 year,month,_:=date.Date()
 first:=time.Date(year,month+1,1,0,0,0,0,time.UTC)
 last:=time.Date(first.Year(),first.Month()+1,0,0,0,0,0,time.UTC).Day()
 if anchor>last{anchor=last}
 return time.Date(first.Year(),first.Month(),anchor,0,0,0,0,time.UTC)
}
func (s *server) rewardBalance(r *http.Request,tx pgx.Tx,user uuid.UUID)(int64,error){
 var balance int64
 err:=tx.QueryRow(r.Context(),"SELECT COALESCE(SUM(delta),0)::bigint FROM family_point_ledger WHERE user_id=$1",user).Scan(&balance)
 return balance,err
}
func (s *server) lockRewardUser(r *http.Request,tx pgx.Tx,id uuid.UUID)error{
 var active bool
 err:=tx.QueryRow(r.Context(),"SELECT active FROM users WHERE id=$1 FOR UPDATE",id).Scan(&active)
 if err!=nil{return err}
 if !active{return errors.New("Inactive family member")}
 return nil
}
// Accrue only established schedules. A unique plan/date key makes catch-up
// idempotent across concurrent client requests or repeated retries.
func (s *server) accrueAllowances(r *http.Request)error {
 tz:="UTC"
 if err:=s.db.QueryRow(r.Context(),
  "SELECT COALESCE((SELECT value FROM app_settings WHERE key='timezone'),'UTC')").Scan(&tz);err!=nil{return err}
 loc,err:=time.LoadLocation(tz);if err!=nil{loc=time.UTC}
 today,_:=choreDate(time.Now().In(loc).Format("2006-01-02"))
 tx,err:=s.db.Begin(r.Context());if err!=nil{return err}
 defer tx.Rollback(r.Context())
 rows,err:=tx.Query(r.Context(),`SELECT id,user_id,amount_cents,frequency,anchor_day,next_due_date,created_by
   FROM family_allowance_plans WHERE active=true ORDER BY id FOR UPDATE`)
 if err!=nil{return err}
 type plan struct {id,user,creator uuid.UUID;amount int64;frequency string;anchor int;next time.Time}
 plans:=[]plan{}
 for rows.Next(){
  var p plan
  if err=rows.Scan(&p.id,&p.user,&p.amount,&p.frequency,&p.anchor,&p.next,&p.creator);err!=nil{break}
  plans=append(plans,p)
 }
 if err==nil{err=rows.Err()}
 rows.Close()
 if err!=nil{return err}
 for _,p:=range plans {
  // Generate at most 65 occurrences per open; a long-offline household
  // catches up on later reads without unbounded work.
  due:=p.next
  for i:=0;i<65&&!due.After(today);i++{
   _,err=tx.Exec(r.Context(),`INSERT INTO family_allowance_entries
      (user_id,plan_id,due_date,kind,amount_cents,note,recorded_by)
      VALUES($1,$2,$3::date,'due',$4,'Scheduled allowance',$5)
      ON CONFLICT (plan_id,due_date) WHERE kind='due' DO NOTHING`,
      p.user,p.id,due.Format("2006-01-02"),p.amount,p.creator)
   if err!=nil{return err}
   due=allowanceAdvance(due,p.frequency,p.anchor)
  }
  if !due.Equal(p.next) {
   _,err=tx.Exec(r.Context(),
     "UPDATE family_allowance_plans SET next_due_date=$2::date,updated_at=now() WHERE id=$1",
     p.id,due.Format("2006-01-02"))
   if err!=nil{return err}
  }
 }
 return tx.Commit(r.Context())
}
func (s *server) rewardsOverview(w http.ResponseWriter,r *http.Request){
 if !s.requireRewards(w,r){return}
 if err:=s.accrueAllowances(r);err!=nil {
  writeError(w,500,"Could not update scheduled allowances");return
 }
 a:=currentActor(r);manager:=rewardManager(r)
 catalog:=[]map[string]any{}
 rows,err:=s.db.Query(r.Context(),`SELECT id,title,description,cost_points,active
   FROM family_reward_catalog WHERE active=true OR $1 ORDER BY active DESC,cost_points,title`,manager)
 if err!=nil{writeError(w,500,"Could not load rewards");return}
 for rows.Next(){
  var id uuid.UUID;var title,description string;var cost int;var active bool
  if err=rows.Scan(&id,&title,&description,&cost,&active);err!=nil{break}
  catalog=append(catalog,map[string]any{"id":id,"title":title,"description":description,"cost_points":cost,"active":active})
 }
 if err==nil{err=rows.Err()}
 rows.Close();if err!=nil{writeError(w,500,"Could not read rewards");return}
 balances:=[]map[string]any{}
 rows,err=s.db.Query(r.Context(),`SELECT u.id,u.display_name,
   COALESCE((SELECT SUM(delta) FROM family_point_ledger WHERE user_id=u.id),0)::bigint,
   COALESCE((SELECT SUM(CASE WHEN kind='due' THEN amount_cents ELSE -amount_cents END)
     FROM family_allowance_entries WHERE user_id=u.id),0)::bigint
   FROM users u WHERE u.active=true AND ($1::boolean OR u.id=$2)
   ORDER BY u.display_name`,manager,a.ID)
 if err!=nil{writeError(w,500,"Could not load balances");return}
 for rows.Next(){
  var id uuid.UUID;var name string;var points,cents int64
  if err=rows.Scan(&id,&name,&points,&cents);err!=nil{break}
  balances=append(balances,map[string]any{"user_id":id,"name":name,"points":points,"allowance_balance_cents":cents})
 }
 if err==nil{err=rows.Err()}
 rows.Close();if err!=nil{writeError(w,500,"Could not read balances");return}
 pending:=[]map[string]any{}
 if manager {
  rows,err=s.db.Query(r.Context(),`
   SELECT c.id,c.title,p.user_id,u.display_name,to_char(p.occurrence_date,'YYYY-MM-DD'),c.reward_points
   FROM chore_progress p JOIN chores c ON c.id=p.chore_id
   JOIN users u ON u.id=p.user_id
   LEFT JOIN family_chore_reviews rv ON rv.chore_id=c.id AND rv.user_id=p.user_id AND rv.occurrence_date=p.occurrence_date
   WHERE p.status='done' AND c.reward_points>0 AND c.kind='chore'
   AND (rv.id IS NULL OR rv.decision='rejected') AND u.active=true
   ORDER BY p.occurrence_date DESC,c.title LIMIT 100`)
  if err!=nil{writeError(w,500,"Could not load pending approvals");return}
  for rows.Next(){
   var id,user uuid.UUID;var title,name,date string;var points int
   if err=rows.Scan(&id,&title,&user,&name,&date,&points);err!=nil{break}
   pending=append(pending,map[string]any{"chore_id":id,"title":title,"user_id":user,"name":name,"date":date,"points":points})
  }
  if err==nil{err=rows.Err()}
  rows.Close();if err!=nil{writeError(w,500,"Could not read pending approvals");return}
 }
 claims:=[]map[string]any{}
 rows,err=s.db.Query(r.Context(),`SELECT id,user_id,title_snapshot,cost_points,status,requested_at
   FROM family_reward_claims WHERE ($1::boolean OR user_id=$2)
   ORDER BY requested_at DESC LIMIT 120`,manager,a.ID)
 if err!=nil{writeError(w,500,"Could not load reward requests");return}
 for rows.Next(){
  var id,user uuid.UUID;var title,status string;var cost int;var date time.Time
  if err=rows.Scan(&id,&user,&title,&cost,&status,&date);err!=nil{break}
  claims=append(claims,map[string]any{"id":id,"user_id":user,"title":title,"cost_points":cost,"status":status,"requested_at":date})
 }
 if err==nil{err=rows.Err()}
 rows.Close();if err!=nil{writeError(w,500,"Could not load reward requests");return}
 plans:=[]map[string]any{}
 if manager {
  rows,err=s.db.Query(r.Context(),`SELECT id,user_id,amount_cents,frequency,next_due_date,active
    FROM family_allowance_plans ORDER BY user_id`)
  if err!=nil{writeError(w,500,"Could not load allowance plans");return}
  for rows.Next(){
   var id,user uuid.UUID;var amount int64;var frequency string;var next time.Time;var active bool
   if err=rows.Scan(&id,&user,&amount,&frequency,&next,&active);err!=nil{break}
   plans=append(plans,map[string]any{"id":id,"user_id":user,"amount_cents":amount,"frequency":frequency,"next_due_date":next.Format("2006-01-02"),"active":active})
  }
  if err==nil{err=rows.Err()}
  rows.Close();if err!=nil{writeError(w,500,"Could not load allowance plans");return}
 }
 ledger:=[]map[string]any{}
 rows,err=s.db.Query(r.Context(),`SELECT 'points' AS ledger_type,user_id,delta::bigint AS amount,description,created_at
   FROM family_point_ledger WHERE ($1::boolean OR user_id=$2)
   UNION ALL
   SELECT 'allowance' AS ledger_type,user_id,
     CASE WHEN kind='due' THEN amount_cents ELSE -amount_cents END,
     CASE WHEN kind='due' THEN 'Allowance due' ELSE 'Allowance payment recorded' END || ': ' || note,
     created_at FROM family_allowance_entries WHERE ($1::boolean OR user_id=$2)
   ORDER BY created_at DESC LIMIT 120`,manager,a.ID)
 if err!=nil{writeError(w,500,"Could not load reward activity");return}
 for rows.Next(){
  var kind,description string;var user uuid.UUID;var amount int64;var date time.Time
  if err=rows.Scan(&kind,&user,&amount,&description,&date);err!=nil{break}
  ledger=append(ledger,map[string]any{"type":kind,"user_id":user,"amount":amount,"description":description,"created_at":date})
 }
 if err==nil{err=rows.Err()}
 rows.Close();if err!=nil{writeError(w,500,"Could not load reward activity");return}
 writeJSON(w,200,map[string]any{"catalog":catalog,"balances":balances,"pending_chores":pending,
  "claims":claims,"plans":plans,"activity":ledger,"can_manage":manager})
}
func (s *server) rewardCatalogCreate(w http.ResponseWriter,r *http.Request){
 if !s.requireRewards(w,r)||!requiredRewardManager(w,r){return}
 var in rewardCatalogInput
 if decode(r,&in)!=nil{writeError(w,400,"Check reward details");return}
 var ok bool
 if in,ok=validRewardCatalog(in);!ok{writeError(w,400,"Enter a reward name and positive points cost");return}
 var id uuid.UUID
 err:=s.db.QueryRow(r.Context(),`INSERT INTO family_reward_catalog
   (title,description,cost_points,active,created_by) VALUES($1,$2,$3,$4,$5) RETURNING id`,
   in.Title,in.Description,in.CostPoints,in.Active,currentActor(r).ID).Scan(&id)
 if err!=nil{writeError(w,500,"Could not save reward");return}
 s.audit(r,"create","reward",&id,"Created household reward",nil)
 writeJSON(w,201,map[string]any{"id":id})
}
func (s *server) rewardCatalogUpdate(w http.ResponseWriter,r *http.Request){
 if !s.requireRewards(w,r)||!requiredRewardManager(w,r){return}
 id,err:=uuid.Parse(r.PathValue("id"));if err!=nil{writeError(w,400,"Invalid reward");return}
 var in rewardCatalogInput
 if decode(r,&in)!=nil{writeError(w,400,"Check reward details");return}
 var ok bool
 if in,ok=validRewardCatalog(in);!ok{writeError(w,400,"Enter a reward name and positive points cost");return}
 tag,err:=s.db.Exec(r.Context(),`UPDATE family_reward_catalog SET title=$2,description=$3,
   cost_points=$4,active=$5,updated_at=now() WHERE id=$1`,
   id,in.Title,in.Description,in.CostPoints,in.Active)
 if err!=nil{writeError(w,500,"Could not update reward");return}
 if tag.RowsAffected()==0{writeError(w,404,"Reward not found");return}
 s.audit(r,"update","reward",&id,"Updated household reward",nil)
 writeJSON(w,200,map[string]any{"id":id})
}
func (s *server) choreReviewReward(w http.ResponseWriter,r *http.Request){
 if !s.requireRewards(w,r)||!requiredRewardManager(w,r){return}
 choreID,err:=uuid.Parse(r.PathValue("id"));if err!=nil{writeError(w,400,"Invalid chore");return}
 var in struct{UserID uuid.UUID `json:"user_id"`;Date string `json:"date"`;Decision string `json:"decision"`;Note string `json:"note"`}
 if decode(r,&in)!=nil||in.UserID==uuid.Nil||(in.Decision!="approved"&&in.Decision!="rejected"){
  writeError(w,400,"Choose an assignee and an approval decision");return
 }
 date,err:=rewardDate(in.Date);if err!=nil{writeError(w,400,"Choose a valid chore date");return}
 in.Note=strings.TrimSpace(in.Note);if len(in.Note)>1000{writeError(w,400,"Keep review notes below 1000 characters");return}
 tx,err:=s.db.Begin(r.Context());if err!=nil{writeError(w,500,"Could not review chore");return}
 defer tx.Rollback(r.Context())
 var points int;var title,kind string
 err=tx.QueryRow(r.Context(),`SELECT title,kind,reward_points FROM chores WHERE id=$1 FOR UPDATE`,choreID).Scan(&title,&kind,&points)
 if errors.Is(err,pgx.ErrNoRows){writeError(w,404,"Chore not found");return}
 if err!=nil{writeError(w,500,"Could not load chore");return}
 if kind!="chore"||points<=0{writeError(w,400,"This chore has no points reward");return}
 var done bool
 err=tx.QueryRow(r.Context(),`SELECT EXISTS(SELECT 1 FROM chore_progress
  WHERE chore_id=$1 AND user_id=$2 AND occurrence_date=$3::date AND status='done')`,
  choreID,in.UserID,date.Format("2006-01-02")).Scan(&done)
 if err!=nil{writeError(w,500,"Could not check completion");return}
 if !done{writeError(w,409,"The assigned chore must be completed before review");return}
 var reviewID uuid.UUID;var existing string
 err=tx.QueryRow(r.Context(),`SELECT id,decision FROM family_chore_reviews
  WHERE chore_id=$1 AND user_id=$2 AND occurrence_date=$3::date FOR UPDATE`,
  choreID,in.UserID,date.Format("2006-01-02")).Scan(&reviewID,&existing)
 if err!=nil&&!errors.Is(err,pgx.ErrNoRows){writeError(w,500,"Could not load prior review");return}
 if err==nil&&existing=="approved"{writeJSON(w,200,map[string]any{"id":reviewID,"already_approved":true});return}
 if err==nil {
  _,err=tx.Exec(r.Context(),`UPDATE family_chore_reviews SET decision=$2,points_awarded=$3,
   reviewed_by=$4,note=$5,reviewed_at=now() WHERE id=$1`,
   reviewID,in.Decision,func()int{if in.Decision=="approved"{return points};return 0}(),currentActor(r).ID,in.Note)
 }else{
  err=tx.QueryRow(r.Context(),`INSERT INTO family_chore_reviews
    (chore_id,user_id,occurrence_date,decision,points_awarded,reviewed_by,note)
    VALUES($1,$2,$3::date,$4,$5,$6,$7) RETURNING id`,
   choreID,in.UserID,date.Format("2006-01-02"),in.Decision,
   func()int{if in.Decision=="approved"{return points};return 0}(),currentActor(r).ID,in.Note).Scan(&reviewID)
 }
 if err!=nil{writeError(w,500,"Could not save review");return}
 if in.Decision=="rejected" {
  // Return the work to the assignee. Their next completion reappears in the
  // approval queue; the rejected review remains as an audit record.
  _,err=tx.Exec(r.Context(),`UPDATE chore_progress SET status='in_progress',
     completed_at=NULL,completed_by=NULL,updated_at=now()
     WHERE chore_id=$1 AND user_id=$2 AND occurrence_date=$3::date`,
     choreID,in.UserID,date.Format("2006-01-02"))
  if err!=nil{writeError(w,500,"Could not return chore for correction");return}
  var primary uuid.UUID;var start time.Time
  err=tx.QueryRow(r.Context(),"SELECT assigned_to,due_date FROM chores WHERE id=$1",
     choreID).Scan(&primary,&start)
  if err!=nil{writeError(w,500,"Could not update chore summary");return}
  if primary==in.UserID && start.Format("2006-01-02")==date.Format("2006-01-02") {
   _,err=tx.Exec(r.Context(),`UPDATE chores SET status='in_progress',
      completed_at=NULL,completed_by=NULL,updated_at=now() WHERE id=$1`,choreID)
   if err!=nil{writeError(w,500,"Could not return chore summary");return}
  }
 }
 if in.Decision=="approved"{
  if err=s.lockRewardUser(r,tx,in.UserID);err!=nil{writeError(w,400,"Assignee is not active");return}
  _,err=tx.Exec(r.Context(),`INSERT INTO family_point_ledger
   (user_id,delta,kind,source_id,description,created_by)
   VALUES($1,$2,'chore',$3,$4,$5) ON CONFLICT (kind,source_id) WHERE source_id IS NOT NULL DO NOTHING`,
   in.UserID,points,reviewID,"Chore approved: "+title,currentActor(r).ID)
  if err!=nil{writeError(w,500,"Could not credit chore points");return}
 }
 if err=tx.Commit(r.Context());err!=nil{writeError(w,500,"Could not finish chore review");return}
 s.audit(r,"review","chore",&choreID,"Reviewed chore for reward",map[string]any{"user_id":in.UserID,"decision":in.Decision})
 writeJSON(w,200,map[string]any{"id":reviewID,"decision":in.Decision})
}
func (s *server) pointAdjustment(w http.ResponseWriter,r *http.Request){
 if !s.requireRewards(w,r)||!requiredRewardManager(w,r){return}
 var in struct{UserID uuid.UUID `json:"user_id"`;Delta int `json:"delta"`;Reason string `json:"reason"`;RequestID uuid.UUID `json:"request_id"`}
 if decode(r,&in)!=nil||in.UserID==uuid.Nil||in.RequestID==uuid.Nil||in.Delta==0||in.Delta < -10000||in.Delta>10000{
  writeError(w,400,"Choose a person, signed point amount and request ID");return
 }
 in.Reason=strings.TrimSpace(in.Reason);if len(in.Reason)<3||len(in.Reason)>250{
  writeError(w,400,"Give a reason for the adjustment (3–250 characters)");return
 }
 tx,err:=s.db.Begin(r.Context());if err!=nil{writeError(w,500,"Could not update points");return}
 defer tx.Rollback(r.Context())
 if err=s.lockRewardUser(r,tx,in.UserID);err!=nil{writeError(w,400,"Person not found or inactive");return}
 var existingID,user uuid.UUID;var oldDelta int;var oldReason string
 err=tx.QueryRow(r.Context(),`SELECT id,user_id,delta,description FROM family_point_ledger
  WHERE kind='adjustment' AND source_id=$1`,in.RequestID).Scan(&existingID,&user,&oldDelta,&oldReason)
 if err==nil {
  if user!=in.UserID||oldDelta!=in.Delta||oldReason!=in.Reason{writeError(w,409,"Request ID already used for another adjustment");return}
  writeJSON(w,200,map[string]any{"id":existingID,"duplicate":true});return
 }
 if !errors.Is(err,pgx.ErrNoRows){writeError(w,500,"Could not check adjustment request");return}
 balance,err:=s.rewardBalance(r,tx,in.UserID)
 if err!=nil{writeError(w,500,"Could not check points balance");return}
 if balance+int64(in.Delta)<0{writeError(w,409,"Not enough points for this adjustment");return}
 var id uuid.UUID
 err=tx.QueryRow(r.Context(),`INSERT INTO family_point_ledger
  (user_id,delta,kind,source_id,description,created_by)
  VALUES($1,$2,'adjustment',$3,$4,$5) RETURNING id`,
  in.UserID,in.Delta,in.RequestID,in.Reason,currentActor(r).ID).Scan(&id)
 if err!=nil{writeError(w,500,"Could not record adjustment");return}
 if err=tx.Commit(r.Context());err!=nil{writeError(w,500,"Could not save adjustment");return}
 s.audit(r,"adjust","points",&id,"Adjusted reward points",map[string]any{"person_id":in.UserID,"delta":in.Delta})
 writeJSON(w,201,map[string]any{"id":id})
}
func (s *server) claimReward(w http.ResponseWriter,r *http.Request){
 if !s.requireRewards(w,r){return}
 rewardID,err:=uuid.Parse(r.PathValue("id"));if err!=nil{writeError(w,400,"Invalid reward");return}
 var in struct{RequestID uuid.UUID `json:"request_id"`}
 if decode(r,&in)!=nil||in.RequestID==uuid.Nil{writeError(w,400,"A unique request ID is required");return}
 actor:=currentActor(r)
 tx,err:=s.db.Begin(r.Context());if err!=nil{writeError(w,500,"Could not request reward");return}
 defer tx.Rollback(r.Context())
 if err=s.lockRewardUser(r,tx,actor.ID);err!=nil{writeError(w,403,"Account is inactive");return}
 var oldID,oldUser,oldReward uuid.UUID
 err=tx.QueryRow(r.Context(),"SELECT id,user_id,reward_id FROM family_reward_claims WHERE request_id=$1",in.RequestID).Scan(&oldID,&oldUser,&oldReward)
 if err==nil {
  if oldUser!=actor.ID||oldReward!=rewardID{writeError(w,409,"Request ID is already in use");return}
  writeJSON(w,200,map[string]any{"id":oldID,"duplicate":true});return
 }
 if !errors.Is(err,pgx.ErrNoRows){writeError(w,500,"Could not check reward request");return}
 var title string;var cost int
 err=tx.QueryRow(r.Context(),"SELECT title,cost_points FROM family_reward_catalog WHERE id=$1 AND active=true",rewardID).Scan(&title,&cost)
 if errors.Is(err,pgx.ErrNoRows){writeError(w,404,"Reward not found");return}
 if err!=nil{writeError(w,500,"Could not load reward");return}
 balance,err:=s.rewardBalance(r,tx,actor.ID)
 if err!=nil{writeError(w,500,"Could not check points balance");return}
 if balance<int64(cost){writeError(w,409,"Not enough points for this reward");return}
 var id uuid.UUID
 err=tx.QueryRow(r.Context(),`INSERT INTO family_reward_claims
   (user_id,reward_id,title_snapshot,cost_points,request_id)
   VALUES($1,$2,$3,$4,$5) RETURNING id`,
   actor.ID,rewardID,title,cost,in.RequestID).Scan(&id)
 if err!=nil{writeError(w,500,"Could not request reward");return}
 _,err=tx.Exec(r.Context(),`INSERT INTO family_point_ledger
   (user_id,delta,kind,source_id,description,created_by)
   VALUES($1,$2,'redemption',$3,$4,$1)`,
   actor.ID,-cost,id,"Requested reward: "+title)
 if err!=nil{writeError(w,500,"Could not reserve reward points");return}
 if err=tx.Commit(r.Context());err!=nil{writeError(w,500,"Could not save reward request");return}
 s.audit(r,"redeem","reward",&id,"Requested household reward",nil)
 writeJSON(w,201,map[string]any{"id":id,"status":"pending"})
}
func (s *server) resolveRewardClaim(w http.ResponseWriter,r *http.Request){
 if !s.requireRewards(w,r)||!requiredRewardManager(w,r){return}
 id,err:=uuid.Parse(r.PathValue("id"));if err!=nil{writeError(w,400,"Invalid reward request");return}
 var in struct{Status string `json:"status"`}
 if decode(r,&in)!=nil||(in.Status!="fulfilled"&&in.Status!="cancelled"){
  writeError(w,400,"Choose Fulfilled or Cancelled");return
 }
 tx,err:=s.db.Begin(r.Context());if err!=nil{writeError(w,500,"Could not update reward request");return}
 defer tx.Rollback(r.Context())
 // Lock user first: same order as redemption and all points adjustments.
 var owner uuid.UUID
 err=tx.QueryRow(r.Context(),"SELECT user_id FROM family_reward_claims WHERE id=$1",id).Scan(&owner)
 if errors.Is(err,pgx.ErrNoRows){writeError(w,404,"Reward request not found");return}
 if err!=nil{writeError(w,500,"Could not load reward request");return}
 if err=s.lockRewardUser(r,tx,owner);err!=nil{writeError(w,400,"Reward owner is inactive");return}
 var title,status string;var cost int
 err=tx.QueryRow(r.Context(),`SELECT title_snapshot,cost_points,status FROM family_reward_claims
   WHERE id=$1 FOR UPDATE`,id).Scan(&title,&cost,&status)
 if err!=nil{writeError(w,500,"Could not lock reward request");return}
 if status==in.Status{writeJSON(w,200,map[string]any{"id":id,"status":status,"duplicate":true});return}
 if status!="pending"{writeError(w,409,"This reward request has already been resolved");return}
 _,err=tx.Exec(r.Context(),`UPDATE family_reward_claims
  SET status=$2,reviewed_by=$3,reviewed_at=now() WHERE id=$1`,id,in.Status,currentActor(r).ID)
 if err!=nil{writeError(w,500,"Could not update reward request");return}
 if in.Status=="cancelled"{
  _,err=tx.Exec(r.Context(),`INSERT INTO family_point_ledger
   (user_id,delta,kind,source_id,description,created_by)
   VALUES($1,$2,'refund',$3,$4,$5)`,owner,cost,id,"Cancelled reward: "+title,currentActor(r).ID)
  if err!=nil{writeError(w,500,"Could not return reward points");return}
 }
 if err=tx.Commit(r.Context());err!=nil{writeError(w,500,"Could not save reward decision");return}
 s.audit(r,"review","reward",&id,"Updated reward request",map[string]any{"status":in.Status})
 writeJSON(w,200,map[string]any{"id":id,"status":in.Status})
}
type allowancePlanInput struct{
 UserID uuid.UUID `json:"user_id"`
 AmountCents int64 `json:"amount_cents"`
 Frequency string `json:"frequency"`
 FirstDue string `json:"first_due"`
 Active bool `json:"active"`
}
func validAllowancePlan(in allowancePlanInput)bool{
 if in.UserID==uuid.Nil||in.AmountCents<1||in.AmountCents>100000000{return false}
 if in.Frequency!="weekly"&&in.Frequency!="monthly"{return false}
 _,err:=choreDate(in.FirstDue)
 return err==nil
}
func (s *server) saveAllowancePlan(w http.ResponseWriter,r *http.Request){
 if !s.requireRewards(w,r)||!requiredRewardManager(w,r){return}
 var in allowancePlanInput
 if decode(r,&in)!=nil||!validAllowancePlan(in){
  writeError(w,400,"Check person, amount, weekly/monthly schedule and first date");return
 }
 first,_:=choreDate(in.FirstDue)
 // Reject plans which would require enormous synchronous historical accrual.
 now:=time.Now().UTC()
 if first.Before(now.AddDate(-1,0,0))||first.After(now.AddDate(3,0,0)){
  writeError(w,400,"Choose a first date within one year ago and three years ahead");return
 }
 tx,err:=s.db.Begin(r.Context());if err!=nil{writeError(w,500,"Could not save allowance plan");return}
 defer tx.Rollback(r.Context())
 if err=s.lockRewardUser(r,tx,in.UserID);err!=nil{writeError(w,400,"Choose an active family member");return}
 var id uuid.UUID
 err=tx.QueryRow(r.Context(),`INSERT INTO family_allowance_plans
  (user_id,amount_cents,frequency,anchor_day,next_due_date,active,created_by)
  VALUES($1,$2,$3,$4,$5::date,$6,$7)
  ON CONFLICT(user_id) DO UPDATE SET amount_cents=EXCLUDED.amount_cents,
   frequency=EXCLUDED.frequency,anchor_day=EXCLUDED.anchor_day,
   next_due_date=EXCLUDED.next_due_date,active=EXCLUDED.active,updated_at=now()
  RETURNING id`,in.UserID,in.AmountCents,in.Frequency,first.Day(),in.FirstDue,in.Active,currentActor(r).ID).Scan(&id)
 if err!=nil{writeError(w,500,"Could not save allowance plan");return}
 if err=tx.Commit(r.Context());err!=nil{writeError(w,500,"Could not save allowance plan");return}
 s.audit(r,"update","allowance_plan",&id,"Configured repeating allowance",nil)
 writeJSON(w,200,map[string]any{"id":id})
}
func (s *server) recordAllowancePayment(w http.ResponseWriter,r *http.Request){
 if !s.requireRewards(w,r)||!requiredRewardManager(w,r){return}
 var in struct{UserID uuid.UUID `json:"user_id"`;AmountCents int64 `json:"amount_cents"`;Note string `json:"note"`;RequestID uuid.UUID `json:"request_id"`}
 if decode(r,&in)!=nil||in.UserID==uuid.Nil||in.RequestID==uuid.Nil||
  in.AmountCents<1||in.AmountCents>100000000{
  writeError(w,400,"Choose a person, payment amount and request ID");return
 }
 in.Note=strings.TrimSpace(in.Note);if len(in.Note)>300{writeError(w,400,"Note is too long");return}
 tx,err:=s.db.Begin(r.Context());if err!=nil{writeError(w,500,"Could not record allowance payment");return}
 defer tx.Rollback(r.Context())
 if err=s.lockRewardUser(r,tx,in.UserID);err!=nil{writeError(w,400,"Choose an active family member");return}
 var oldID,oldUser uuid.UUID;var oldAmount int64;var oldNote string
 err=tx.QueryRow(r.Context(),`SELECT id,user_id,amount_cents,note FROM family_allowance_entries
   WHERE request_id=$1`,in.RequestID).Scan(&oldID,&oldUser,&oldAmount,&oldNote)
 if err==nil{
  if oldUser!=in.UserID||oldAmount!=in.AmountCents||oldNote!=in.Note{writeError(w,409,"Request ID used for a different payment");return}
  writeJSON(w,200,map[string]any{"id":oldID,"duplicate":true});return
 }
 if !errors.Is(err,pgx.ErrNoRows){writeError(w,500,"Could not check payment request");return}
 var id uuid.UUID
 err=tx.QueryRow(r.Context(),`INSERT INTO family_allowance_entries
   (user_id,kind,amount_cents,note,request_id,recorded_by)
   VALUES($1,'paid',$2,$3,$4,$5) RETURNING id`,
   in.UserID,in.AmountCents,in.Note,in.RequestID,currentActor(r).ID).Scan(&id)
 if err!=nil{writeError(w,500,"Could not record payment");return}
 if err=tx.Commit(r.Context());err!=nil{writeError(w,500,"Could not save allowance payment");return}
 s.audit(r,"record_payment","allowance",&id,"Recorded allowance payment",nil)
 writeJSON(w,201,map[string]any{"id":id})
}
