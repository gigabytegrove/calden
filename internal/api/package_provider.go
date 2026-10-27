package api

import (
 "context"
 "errors"
 "net/http"
 "strings"
 "time"

 "github.com/gigabytegrove/calden/internal/secrets"
 "github.com/google/uuid"
 "github.com/jackc/pgx/v5"
 "github.com/jackc/pgx/v5/pgxpool"
)

const trackingSecretPrefix="calden-aftership-v1:"
const trackingPollInterval=3*time.Hour

func(s *server) trackingProviderKey(ctx context.Context)(string,error){
 var sealed string
 var active bool
 err:=s.db.QueryRow(ctx,"SELECT sealed_api_key,enabled FROM package_tracking_provider WHERE id=1").Scan(&sealed,&active)
 if errors.Is(err,pgx.ErrNoRows){return "",nil}
 if err!=nil{return "",err}
 if !active{return "",nil}
 raw,err:=secrets.Open(s.jwtSecret,sealed)
 if err!=nil||!strings.HasPrefix(raw,trackingSecretPrefix){return "",errors.New("Could not decrypt tracking provider key")}
 return strings.TrimPrefix(raw,trackingSecretPrefix),nil
}
func(s *server) trackingProviderInfo(w http.ResponseWriter,r *http.Request){
 if currentActor(r).Role!="admin"{writeError(w,403,"Admin access required");return}
 var configured,enabled bool
 err:=s.db.QueryRow(r.Context(),"SELECT EXISTS(SELECT 1 FROM package_tracking_provider WHERE id=1),COALESCE((SELECT enabled FROM package_tracking_provider WHERE id=1),false)").Scan(&configured,&enabled)
 if err!=nil{writeError(w,500,"Could not load provider settings");return}
 writeJSON(w,200,map[string]any{"configured":configured,"enabled":enabled,"provider":"AfterShip","interval_hours":3})
}
func(s *server) saveTrackingProvider(w http.ResponseWriter,r *http.Request){
 if currentActor(r).Role!="admin"{writeError(w,403,"Admin access required");return}
 var in struct{
  APIKey string `json:"api_key"`
  Enabled bool `json:"enabled"`
 }
 if decode(r,&in)!=nil{writeError(w,400,"Invalid tracking API settings");return}
 in.APIKey=strings.TrimSpace(in.APIKey)
 if len(in.APIKey)>512||strings.ContainsAny(in.APIKey,"\r\n\x00"){writeError(w,400,"Invalid tracking API key");return}
 var sealed string
 err:=s.db.QueryRow(r.Context(),"SELECT sealed_api_key FROM package_tracking_provider WHERE id=1").Scan(&sealed)
 if err!=nil&&!errors.Is(err,pgx.ErrNoRows){writeError(w,500,"Cannot load saved tracking credentials");return}
 if in.APIKey!=""{
  if len(in.APIKey)<12{writeError(w,400,"Tracking API key is too short");return}
  sealed,err=secrets.Seal(s.jwtSecret,trackingSecretPrefix+in.APIKey)
  if err!=nil{writeError(w,500,"Unable to encrypt API key");return}
 }
 if sealed==""{writeError(w,400,"Enter an authorized AfterShip API key");return}
 tx,err:=s.db.Begin(r.Context());if err!=nil{writeError(w,500,"Could not update tracking configuration");return}
 defer tx.Rollback(r.Context())
 var oldSealed string
 e:=tx.QueryRow(r.Context(),"SELECT sealed_api_key FROM package_tracking_provider WHERE id=1 FOR UPDATE").Scan(&oldSealed)
 if e!=nil&&!errors.Is(e,pgx.ErrNoRows){writeError(w,500,"Could not lock tracking configuration");return}
 _,err=tx.Exec(r.Context(),`INSERT INTO package_tracking_provider(id,sealed_api_key,enabled) VALUES(1,$1,$2)
 ON CONFLICT(id) DO UPDATE SET sealed_api_key=EXCLUDED.sealed_api_key,enabled=EXCLUDED.enabled,updated_at=now()`,sealed,in.Enabled)
 if err!=nil{writeError(w,500,"Could not save tracking API settings");return}
 // Provider IDs are per-account; a new key invalidates old AfterShip tracking references.
 if oldSealed!=sealed{
  _,err=tx.Exec(r.Context(),`UPDATE package_shipments SET provider_tracking_id='',provider_slug='',next_tracking_check_at=now(),tracking_error=''
  WHERE NOT archived AND status!='delivered'`)
 }else if in.Enabled{
  _,err=tx.Exec(r.Context(),`UPDATE package_shipments SET next_tracking_check_at=now()
  WHERE NOT archived AND status!='delivered' AND provider_tracking_id=''`)
 }
 if err!=nil{writeError(w,500,"Could not schedule provider refresh");return}
 if err=tx.Commit(r.Context());err!=nil{writeError(w,500,"Could not save tracking API settings");return}
 s.audit(r,"update","package_tracking_provider",nil,"Updated carrier tracking provider settings",nil)
 writeJSON(w,200,map[string]any{"configured":true,"enabled":in.Enabled,"provider":"AfterShip","interval_hours":3})
}
func(s *server) deleteTrackingProvider(w http.ResponseWriter,r *http.Request){
 if currentActor(r).Role!="admin"{writeError(w,403,"Admin access required");return}
 _,err:=s.db.Exec(r.Context(),"DELETE FROM package_tracking_provider WHERE id=1")
 if err!=nil{writeError(w,500,"Could not disconnect provider");return}
 s.audit(r,"delete","package_tracking_provider",nil,"Disconnected carrier tracking",nil)
 w.WriteHeader(204)
}
type dueTracking struct {
 ID uuid.UUID
 Carrier,Number,ProviderID string
}
func claimTracking(ctx context.Context,db *pgxpool.Pool,id uuid.UUID)(dueTracking,bool,error){
 var job dueTracking
 err:=db.QueryRow(ctx,`UPDATE package_shipments SET next_tracking_check_at=now()+interval '3 hours'
 WHERE id=$1 AND NOT archived AND status!='delivered' AND next_tracking_check_at<=now()
 AND COALESCE((SELECT value='true' FROM app_settings WHERE key='packages_enabled'),false)
 AND COALESCE((SELECT enabled FROM package_tracking_provider WHERE id=1),false)
 RETURNING id,carrier,tracking_number,provider_tracking_id`,id).Scan(&job.ID,&job.Carrier,&job.Number,&job.ProviderID)
 if errors.Is(err,pgx.ErrNoRows){return job,false,nil}
 return job,err==nil,err
}
func (s *server) syncPackageTracking(ctx context.Context,job dueTracking,key string)error{
 api:=newAftershipAPI(key)
 var tracking aftershipTracking
 var err error
 if job.ProviderID!=""{tracking,err=api.fetch(ctx,job.ProviderID)
 }else{tracking,err=api.register(ctx,job.ID,job.Number,job.Carrier)}
 if err!=nil{
  // Safe, intentionally generic error; no credentials or provider response bodies.
  _,_=s.db.Exec(ctx,`UPDATE package_shipments SET last_tracking_check_at=now(),tracking_error=$2
   WHERE id=$1 AND tracking_number=$3`,job.ID,err.Error(),job.Number)
  return err
 }
 if tracking.TrackingNumber!=""&&strings.ToUpper(strings.TrimSpace(tracking.TrackingNumber))!=job.Number{
  _,_=s.db.Exec(ctx,`UPDATE package_shipments SET last_tracking_check_at=now(),tracking_error='Provider returned a different tracking number'
   WHERE id=$1 AND tracking_number=$2`,job.ID,job.Number)
  return errors.New("Provider tracking number mismatch")
 }
 status:=providerStatus(tracking.Tag)
 carrier:=providerCourier(strings.ToLower(tracking.Slug))
 if carrier=="other" {carrier=job.Carrier}
 date:=tracking.deliveryDate()
 tx,err:=s.db.Begin(ctx);if err!=nil{return err}
 defer tx.Rollback(ctx)
 // A concurrent user edit may have replaced the number. Never apply stale scans.
 tag,err:=tx.Exec(ctx,`UPDATE package_shipments SET
  provider_tracking_id=$3,provider_slug=$4,carrier=$5,
  status=CASE WHEN $6='' THEN status ELSE $6 END,
  status_source=CASE WHEN $6='' THEN status_source ELSE 'provider' END,
  expected_date=COALESCE(NULLIF($7,'')::date,expected_date),
  delivered_at=CASE WHEN $6='delivered' THEN COALESCE(delivered_at,now()) ELSE delivered_at END,
  tracking_error='',last_tracking_check_at=now(),updated_at=now()
 WHERE id=$1 AND tracking_number=$2 AND NOT archived`,
 job.ID,job.Number,tracking.ID,tracking.Slug,carrier,status,date)
 if err!=nil{return err}
 if tag.RowsAffected()!=1{return errors.New("Package changed during tracking request")}
 for _,cp:=range tracking.Checkpoints{
  if strings.EqualFold(cp.Source,"user"){continue}
  mapped:=providerStatus(cp.Tag)
  if mapped==""{continue}
  detail:=cleanText(strings.TrimSpace(cp.Message),480)
  if detail==""{detail="Carrier reported "+cp.Tag}
  occurred:=cp.occurredAt()
  _,err=tx.Exec(ctx,`INSERT INTO package_checkpoints(shipment_id,status,detail,source,occurred_at,provider_ref)
  VALUES($1,$2,$3,'provider',$4,$5) ON CONFLICT DO NOTHING`,job.ID,mapped,detail,occurred,cp.reference())
  if err!=nil{return err}
 }
 // Always preserve the current carrier status as a checkpoint, even if the
 // provider omitted its checkpoint history. Stable reference avoids duplicates.
 if status!=""{
  ref:="aftership:current:"+tracking.Tag+":"+tracking.SubtagMessage
  if len(ref)>230{ref=ref[:230]}
  _,err=tx.Exec(ctx,`INSERT INTO package_checkpoints(shipment_id,status,detail,source,provider_ref)
  VALUES($1,$2,$3,'provider',$4) ON CONFLICT DO NOTHING`,
  job.ID,status,cleanText(strings.TrimSpace(tracking.SubtagMessage),480),ref)
  if err!=nil{return err}
 }
 return tx.Commit(ctx)
}
func(s *server) refreshPackageTracking(w http.ResponseWriter,r *http.Request){
 id,ok:=s.packageByID(w,r,true);if !ok{return}
 key,err:=s.trackingProviderKey(r.Context())
 if err!=nil{writeError(w,500,"Tracking provider settings are unavailable");return}
 if key==""{writeError(w,409,"Configure and enable an AfterShip tracking API key in Packages settings");return}
 job,claimed,err:=claimTracking(r.Context(),s.db,id)
 if err!=nil{writeError(w,500,"Could not schedule tracking lookup");return}
 if !claimed{writeJSON(w,200,map[string]any{"status":"scheduled","message":"Tracking is already current or scheduled; requests run at most every three hours"});return}
 ctx,cancel:=context.WithTimeout(r.Context(),18*time.Second);defer cancel()
 if err=s.syncPackageTracking(ctx,job,key);err!=nil{writeError(w,502,err.Error());return}
 writeJSON(w,200,map[string]string{"status":"updated"})
}
func StartPackageTracking(ctx context.Context,db *pgxpool.Pool,master []byte){
 go func(){
  timer:=time.NewTicker(time.Minute);defer timer.Stop()
  for{
   select{case <-ctx.Done():return;case <-timer.C:
    processDuePackageTracking(ctx,db,master)
   }
  }
 }()
}
func processDuePackageTracking(ctx context.Context,db *pgxpool.Pool,master []byte){
 s:=&server{db:db,jwtSecret:master}
 key,err:=s.trackingProviderKey(ctx);if err!=nil||key==""{return}
 rows,err:=db.Query(ctx,`WITH due AS (
 SELECT id FROM package_shipments
 WHERE NOT archived AND status!='delivered' AND next_tracking_check_at<=now()
 AND COALESCE((SELECT value='true' FROM app_settings WHERE key='packages_enabled'),false)
 ORDER BY next_tracking_check_at,id LIMIT 5 FOR UPDATE SKIP LOCKED
 ) UPDATE package_shipments p SET next_tracking_check_at=now()+interval '3 hours'
 FROM due WHERE p.id=due.id RETURNING p.id,p.carrier,p.tracking_number,p.provider_tracking_id`)
 if err!=nil{return}
 jobs:=[]dueTracking{}
 for rows.Next(){var j dueTracking
  if rows.Scan(&j.ID,&j.Carrier,&j.Number,&j.ProviderID)==nil{jobs=append(jobs,j)}
 }
 rows.Close()
 for _,job:=range jobs{
  if ctx.Err()!=nil{return}
  qctx,cancel:=context.WithTimeout(ctx,18*time.Second)
  _=s.syncPackageTracking(qctx,job,key)
  cancel()
  select{case <-ctx.Done():return;case <-time.After(250*time.Millisecond):}
 }
}
