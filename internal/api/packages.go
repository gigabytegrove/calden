package api

import (
 "errors"
 "net/http"
 "net/url"
 "regexp"
 "strconv"
 "strings"
 "time"

 "github.com/google/uuid"
 "github.com/jackc/pgx/v5"
)

type packageInput struct {
 Label string `json:"label"`
 Carrier string `json:"carrier"`
 TrackingNumber string `json:"tracking_number"`
 Status string `json:"status"`
 ExpectedDate string `json:"expected_date"`
 Notes string `json:"notes"`
 Shared bool `json:"shared"`
}

type packageRecord struct {
 ID uuid.UUID `json:"id"`
 OwnerID uuid.UUID `json:"owner_id"`
 Shared bool `json:"shared"`
 Label string `json:"label"`
 Carrier string `json:"carrier"`
 TrackingNumber string `json:"tracking_number"`
 Status string `json:"status"`
 StatusSource string `json:"status_source"`
 ExpectedDate string `json:"expected_date"`
 DeliveredAt string `json:"delivered_at"`
 Notes string `json:"notes"`
 UpdatedAt time.Time `json:"updated_at"`
 TrackingURL string `json:"tracking_url"`
 LastTrackingCheck string `json:"last_tracking_check"`
 NextTrackingCheck string `json:"next_tracking_check"`
 TrackingError string `json:"tracking_error"`
 ProviderSlug string `json:"provider_slug"`
 LatestScan string `json:"latest_scan"`
 CanEdit bool `json:"can_edit"`
}

var packageTrackingPattern=regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 ._-]{3,99}$`)

func validPackageCarrier(c string) bool {
 switch c { case "amazon","usps","ups","fedex","other":return true }
 return false
}
func validPackageStatus(s string) bool {
 switch s { case "unknown","label_created","in_transit","out_for_delivery","delivered","exception":return true }
 return false
}
func preparePackageInput(in *packageInput) bool {
 in.Label=strings.TrimSpace(in.Label)
 in.Carrier=strings.ToLower(strings.TrimSpace(in.Carrier))
 if in.Carrier==""||in.Carrier=="auto"{in.Carrier=detectPackageCarrier(in.TrackingNumber)}
 in.TrackingNumber=normalizePackageTrackingNumber(in.TrackingNumber)
 in.Status=strings.ToLower(strings.TrimSpace(in.Status))
 in.ExpectedDate=strings.TrimSpace(in.ExpectedDate)
 in.Notes=strings.TrimSpace(in.Notes)
 if in.Status=="" {in.Status="unknown"}
 if len(in.Label)<1||len(in.Label)>140||len(in.Notes)>2000||!validPackageCarrier(in.Carrier)||!validPackageStatus(in.Status)||!packageTrackingPattern.MatchString(in.TrackingNumber){return false}
 if in.ExpectedDate!=""{
  d,e:=time.Parse("2006-01-02",in.ExpectedDate)
  if e!=nil||d.Format("2006-01-02")!=in.ExpectedDate{return false}
 }
 return true
}

func packageTrackingURL(carrier,number string)string{
 t:=url.QueryEscape(number)
 switch carrier {
 case "ups":return "https://www.ups.com/track?tracknum="+t
 case "usps":return "https://tools.usps.com/go/TrackConfirmAction?tLabels="+t
 case "fedex":return "https://www.fedex.com/fedextrack/?trknbr="+t
 case "amazon":return "https://www.amazon.com/gp/your-account/order-history"
 default:return ""
 }
}
func (s *server) requirePackages(w http.ResponseWriter,r *http.Request)bool{
 var enabled bool
 err:=s.db.QueryRow(r.Context(),"SELECT COALESCE((SELECT value='true' FROM app_settings WHERE key='packages_enabled'),false)").Scan(&enabled)
 if err!=nil {writeError(w,500,"Could not check package tracking settings");return false}
 if !enabled {writeError(w,403,"Package tracking is disabled by your household administrator");return false}
 return true
}
func packageCanWrite(r *http.Request)bool{return currentActor(r).Role!="restricted"&&!currentActor(r).IsChild}

const packageColumns="id,owner_id,shared,label,carrier,tracking_number,status,status_source,COALESCE(expected_date::text,''),COALESCE(delivered_at::text,''),notes,updated_at,COALESCE(last_tracking_check_at::text,''),COALESCE(next_tracking_check_at::text,''),tracking_error,provider_slug,COALESCE((SELECT c.detail FROM package_checkpoints c WHERE c.shipment_id=package_shipments.id AND c.source='provider' ORDER BY c.occurred_at DESC,c.id DESC LIMIT 1),'')"
func scanPackage(row interface{Scan(...any)error},actorID uuid.UUID)(packageRecord,error){
 var p packageRecord
 err:=row.Scan(&p.ID,&p.OwnerID,&p.Shared,&p.Label,&p.Carrier,&p.TrackingNumber,&p.Status,&p.StatusSource,&p.ExpectedDate,&p.DeliveredAt,&p.Notes,&p.UpdatedAt,&p.LastTrackingCheck,&p.NextTrackingCheck,&p.TrackingError,&p.ProviderSlug,&p.LatestScan)
 p.CanEdit=p.OwnerID==actorID
 p.TrackingURL=packageTrackingURL(p.Carrier,p.TrackingNumber)
 return p,err
}
func (s *server) listPackages(w http.ResponseWriter,r *http.Request){
 if !s.requirePackages(w,r){return}
 actorID:=currentActor(r).ID
 rows,err:=s.db.Query(r.Context(),"SELECT "+packageColumns+" FROM package_shipments WHERE NOT archived AND (owner_id=$1 OR shared) ORDER BY (status='delivered'),expected_date ASC NULLS LAST,updated_at DESC LIMIT 500",actorID)
 if err!=nil{writeError(w,500,"Could not load packages");return}
 defer rows.Close()
 items:=make([]packageRecord,0)
 for rows.Next(){
  p,e:=scanPackage(rows,actorID)
  if e!=nil{writeError(w,500,"Could not read packages");return}
  items=append(items,p)
 }
 if rows.Err()!=nil{writeError(w,500,"Could not read packages");return}
 writeJSON(w,200,map[string]any{"items":items})
}
func (s *server) packageByID(w http.ResponseWriter,r *http.Request,write bool)(uuid.UUID,bool){
 if !s.requirePackages(w,r){return uuid.Nil,false}
 id,err:=uuid.Parse(r.PathValue("id"))
 if err!=nil{writeError(w,400,"Invalid package identifier");return uuid.Nil,false}
 var allowed bool
 query:="SELECT EXISTS(SELECT 1 FROM package_shipments WHERE id=$1 AND NOT archived AND (owner_id=$2 OR shared))"
 if write {
  if !packageCanWrite(r){writeError(w,403,"You cannot edit packages");return uuid.Nil,false}
  query="SELECT EXISTS(SELECT 1 FROM package_shipments WHERE id=$1 AND NOT archived AND owner_id=$2)"
 }
 if err=s.db.QueryRow(r.Context(),query,id,currentActor(r).ID).Scan(&allowed);err!=nil{writeError(w,500,"Could not check package access");return uuid.Nil,false}
 if !allowed{writeError(w,404,"Package not found");return uuid.Nil,false}
 return id,true
}
func (s *server) getPackage(w http.ResponseWriter,r *http.Request){
 id,ok:=s.packageByID(w,r,false);if !ok{return}
 p,err:=scanPackage(s.db.QueryRow(r.Context(),"SELECT "+packageColumns+" FROM package_shipments WHERE id=$1",id),currentActor(r).ID)
 if err!=nil{writeError(w,500,"Could not load package");return}
 writeJSON(w,200,p)
}
func (s *server) createPackage(w http.ResponseWriter,r *http.Request){
 if !s.requirePackages(w,r){return}
 if !packageCanWrite(r){writeError(w,403,"You cannot add packages");return}
 var in packageInput
 if decode(r,&in)!=nil||!preparePackageInput(&in){writeError(w,400,"Enter a package name and valid tracking number");return}
 tx,err:=s.db.Begin(r.Context());if err!=nil{writeError(w,500,"Could not add package");return}
 defer tx.Rollback(r.Context())
 // Older versions only archived packages. Remove an archived match owned by
 // this user so the same tracking number can be added again without retaining
 // invisible, conflicting test records. Active duplicates still return 409.
 _,err=tx.Exec(r.Context(),`DELETE FROM package_shipments
 WHERE owner_id=$1 AND carrier=$2 AND upper(tracking_number)=upper($3) AND archived`,
 currentActor(r).ID,in.Carrier,in.TrackingNumber)
 if err!=nil{writeError(w,500,"Could not clear an old archived package");return}
 var id uuid.UUID
 err=tx.QueryRow(r.Context(),"INSERT INTO package_shipments(owner_id,shared,label,carrier,tracking_number,status,expected_date,notes,delivered_at) VALUES($1,$2,$3,$4,$5,$6,NULLIF($7,'')::date,$8,CASE WHEN $6='delivered' THEN now() ELSE NULL END) ON CONFLICT DO NOTHING RETURNING id",currentActor(r).ID,in.Shared,in.Label,in.Carrier,in.TrackingNumber,in.Status,in.ExpectedDate,in.Notes).Scan(&id)
 if errors.Is(err,pgx.ErrNoRows){writeError(w,409,"This tracking number is already in your packages");return}
 if err!=nil{writeError(w,500,"Could not add package");return}
 _,err=tx.Exec(r.Context(),"INSERT INTO package_checkpoints(shipment_id,actor_id,status,detail,source) VALUES($1,$2,$3,'Package added','manual')",id,currentActor(r).ID,in.Status)
 if err!=nil{writeError(w,500,"Could not save package history");return}
 if err=tx.Commit(r.Context());err!=nil{writeError(w,500,"Could not save package");return}
 s.audit(r,"create","package",&id,"Added a package",nil)
 writeJSON(w,201,map[string]any{"id":id})
}
func (s *server) updatePackage(w http.ResponseWriter,r *http.Request){
 id,ok:=s.packageByID(w,r,true);if !ok{return}
 var in packageInput
 if decode(r,&in)!=nil||!preparePackageInput(&in){writeError(w,400,"Check the package details");return}
 tx,err:=s.db.Begin(r.Context());if err!=nil{writeError(w,500,"Could not update package");return}
 defer tx.Rollback(r.Context())
 var oldStatus string
 err=tx.QueryRow(r.Context(),"SELECT status FROM package_shipments WHERE id=$1 AND owner_id=$2 AND NOT archived FOR UPDATE",id,currentActor(r).ID).Scan(&oldStatus)
 if errors.Is(err,pgx.ErrNoRows){writeError(w,404,"Package not found");return}
 if err!=nil{writeError(w,500,"Could not update package");return}
 tag,err:=tx.Exec(r.Context(),"UPDATE package_shipments SET shared=$2,label=$3,carrier=$4,tracking_number=$5,status=$6,status_source=CASE WHEN status!=$6 THEN 'manual' ELSE status_source END,expected_date=NULLIF($7,'')::date,notes=$8,delivered_at=CASE WHEN $6='delivered' THEN COALESCE(delivered_at,now()) ELSE NULL END,provider_tracking_id=CASE WHEN carrier!=$4 OR tracking_number!=$5 THEN '' ELSE provider_tracking_id END,provider_slug=CASE WHEN carrier!=$4 OR tracking_number!=$5 THEN '' ELSE provider_slug END,next_tracking_check_at=CASE WHEN carrier!=$4 OR tracking_number!=$5 THEN now() ELSE next_tracking_check_at END,tracking_error=CASE WHEN carrier!=$4 OR tracking_number!=$5 THEN '' ELSE tracking_error END,updated_at=now() WHERE id=$1",id,in.Shared,in.Label,in.Carrier,in.TrackingNumber,in.Status,in.ExpectedDate,in.Notes)
 if err!=nil {writeError(w,409,"Unable to save: that tracking number may already exist");return}
 if tag.RowsAffected()!=1{writeError(w,404,"Package not found");return}
 if oldStatus!=in.Status{
  _,err=tx.Exec(r.Context(),"INSERT INTO package_checkpoints(shipment_id,actor_id,status,detail,source) VALUES($1,$2,$3,'Status changed','manual')",id,currentActor(r).ID,in.Status)
  if err!=nil{writeError(w,500,"Could not save package history");return}
 }
 if err=tx.Commit(r.Context());err!=nil{writeError(w,500,"Could not save package");return}
 s.audit(r,"update","package",&id,"Updated a package",nil)
 writeJSON(w,200,map[string]any{"id":id})
}
func (s *server) deletePackage(w http.ResponseWriter,r *http.Request){
 id,ok:=s.packageByID(w,r,true);if !ok{return}
 // The checkpoints foreign key cascades, and the audit log remains separate.
 // Shared viewers cannot delete records that belong to another account.
 tag,err:=s.db.Exec(r.Context(),`DELETE FROM package_shipments
 WHERE id=$1 AND owner_id=$2 AND NOT archived`,id,currentActor(r).ID)
 if err!=nil{writeError(w,500,"Could not delete package");return}
 if tag.RowsAffected()==0{writeError(w,404,"Package not found");return}
 s.audit(r,"delete","package",&id,"Permanently deleted package",nil)
 w.WriteHeader(http.StatusNoContent)
}
func (s *server) packageHistory(w http.ResponseWriter,r *http.Request){
 id,ok:=s.packageByID(w,r,false);if !ok{return}
 // Pagination ensures older carrier events are not silently truncated.
 // Always validate access before querying the shipment's checkpoints.
 offset:=0
 if value:=r.URL.Query().Get("offset");value!=""{
  var err error
  offset,err=strconv.Atoi(value)
  if err!=nil||offset<0||offset>100000{
   writeError(w,400,"Invalid history offset");return
  }
 }
 rows,err:=s.db.Query(r.Context(),`SELECT status,detail,source,occurred_at FROM package_checkpoints
  WHERE shipment_id=$1 ORDER BY occurred_at DESC,id DESC LIMIT 101 OFFSET $2`,id,offset)
 if err!=nil{writeError(w,500,"Could not load tracking history");return}
 defer rows.Close()
 type checkpoint struct {
  Status string `json:"status"`
  Detail string `json:"detail"`
  Source string `json:"source"`
  OccurredAt time.Time `json:"occurred_at"`
 }
 items:=make([]checkpoint,0,101)
 for rows.Next(){var c checkpoint
  if rows.Scan(&c.Status,&c.Detail,&c.Source,&c.OccurredAt)!=nil{writeError(w,500,"Could not load history");return}
  items=append(items,c)
 }
 if rows.Err()!=nil{writeError(w,500,"Could not load tracking history");return}
 hasMore:=len(items)>100
 if hasMore{items=items[:100]}
 writeJSON(w,200,map[string]any{"items":items,"has_more":hasMore,"next_offset":offset+len(items)})
}
