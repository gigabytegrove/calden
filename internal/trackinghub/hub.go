package trackinghub

import (
 "bytes"
 "context"
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "embed"
 "errors"
 "fmt"
 "io"
 "net"
 "net/url"
 "net/http"
 "regexp"
 "strings"
 "sync"
 "time"

 "github.com/google/uuid"
 "github.com/jackc/pgx/v5"
 "github.com/jackc/pgx/v5/pgxpool"
)

//go:embed schema.sql
var schemaFS embed.FS

type Config struct{
 DB *pgxpool.Pool
 AfterShipKey string
 AllowEnrollment bool
 MaxInstallationsPerDay int
 MaxShipmentsPerInstall int
 MaxCallsPerInstallPerDay int
 MaxGlobalCallsPerDay int
 Endpoint string // for tests; production uses the official fixed AfterShip API URL
 HTTPClient *http.Client
}
type Hub struct{cfg Config;providerMu sync.Mutex;nextProviderCall time.Time}
const officialAfterShipURL="https://api.aftership.com/tracking/2026-07/trackings"
var trackingNumberRE=regexp.MustCompile(`^[A-Z0-9]{4,100}$`)
var safeIDRE=regexp.MustCompile(`^[a-zA-Z0-9_-]{1,128}$`)

func New(ctx context.Context,cfg Config)(*Hub,error){
 if cfg.DB==nil{return nil,errors.New("missing tracking hub database")}
 if len(cfg.AfterShipKey)<12{return nil,errors.New("missing operator-owned tracking API key")}
 if cfg.MaxInstallationsPerDay<=0{cfg.MaxInstallationsPerDay=100}
 if cfg.MaxShipmentsPerInstall<=0{cfg.MaxShipmentsPerInstall=50}
 if cfg.MaxCallsPerInstallPerDay<=0{cfg.MaxCallsPerInstallPerDay=400}
 if cfg.MaxGlobalCallsPerDay<=0{cfg.MaxGlobalCallsPerDay=5000}
 if cfg.Endpoint==""{cfg.Endpoint=officialAfterShipURL}
 if cfg.HTTPClient==nil{cfg.HTTPClient=&http.Client{Timeout:14*time.Second,CheckRedirect:func(*http.Request,[]*http.Request)error{return http.ErrUseLastResponse}}}
 schema,err:=schemaFS.ReadFile("schema.sql")
 if err!=nil{return nil,err}
 if _,err=cfg.DB.Exec(ctx,string(schema));err!=nil{return nil,fmt.Errorf("hub schema: %w",err)}
 return &Hub{cfg:cfg},nil
}
func (h *Hub) Handler()http.Handler{
 mux:=http.NewServeMux()
 mux.HandleFunc("GET /healthz",func(w http.ResponseWriter,r *http.Request){writeJSON(w,200,map[string]any{"ok":true})})
 mux.HandleFunc("POST /v1/installations",h.enroll)
 mux.HandleFunc("POST /v1/trackings",h.track)
 return http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  w.Header().Set("Cache-Control","no-store")
  w.Header().Set("X-Content-Type-Options","nosniff")
  mux.ServeHTTP(w,r)
 })
}
func writeJSON(w http.ResponseWriter,code int,value any){
 w.Header().Set("Content-Type","application/json")
 w.WriteHeader(code)
 _=json.NewEncoder(w).Encode(value)
}
func fail(w http.ResponseWriter,code int,message string){writeJSON(w,code,map[string]any{"error":message})}
func decodeJSON(w http.ResponseWriter,r *http.Request,output any)bool{
 r.Body=http.MaxBytesReader(w,r.Body,4096)
 decoder:=json.NewDecoder(r.Body);decoder.DisallowUnknownFields()
 if err:=decoder.Decode(output);err!=nil{fail(w,400,"Invalid tracking request");return false}
 var extra any
 if decoder.Decode(&extra)!=io.EOF{fail(w,400,"Unexpected request content");return false}
 return true
}
func tokenHash(r *http.Request)(string,bool){
 header:=r.Header.Get("Authorization")
 if !strings.HasPrefix(header,"Bearer "){return "",false}
 token:=strings.TrimPrefix(header,"Bearer ")
 if len(token)!=64{return "",false}
 if _,err:=hex.DecodeString(token);err!=nil{return "",false}
 digest:=sha256.Sum256([]byte(token))
 return hex.EncodeToString(digest[:]),true
}
func(h *Hub) validInstallation(ctx context.Context,r *http.Request)(uuid.UUID,bool){
 hash,ok:=tokenHash(r);if !ok{return uuid.Nil,false}
 var id uuid.UUID
 err:=h.cfg.DB.QueryRow(ctx,"SELECT installation_id FROM hub_installations WHERE token_hash=$1 AND enabled",hash).Scan(&id)
 return id,err==nil
}
func charge(ctx context.Context,tx pgx.Tx,subject string,max int)error{
 var count int
 err:=tx.QueryRow(ctx,`INSERT INTO hub_usage(subject,usage_day,request_count) VALUES($1,CURRENT_DATE,1)
  ON CONFLICT(subject,usage_day) DO UPDATE SET request_count=hub_usage.request_count+1
  WHERE hub_usage.request_count<$2 RETURNING request_count`,subject,max).Scan(&count)
 if errors.Is(err,pgx.ErrNoRows){return errors.New("usage quota reached")}
 return err
}
func(h *Hub) enroll(w http.ResponseWriter,r *http.Request){
 if !h.cfg.AllowEnrollment{fail(w,403,"Automatic installation enrollment is not enabled on this hub");return}
 hash,ok:=tokenHash(r);if !ok{fail(w,401,"Installation credential required");return}
 var body struct{InstallationID string `json:"installation_id"`}
 if !decodeJSON(w,r,&body){return}
 id,err:=uuid.Parse(body.InstallationID);if err!=nil||id==uuid.Nil{fail(w,400,"Invalid installation identity");return}
 ctx,cancel:=context.WithTimeout(r.Context(),5*time.Second);defer cancel()
 tx,err:=h.cfg.DB.Begin(ctx);if err!=nil{fail(w,503,"Tracking hub unavailable");return}
 defer tx.Rollback(ctx)
 var existing uuid.UUID
 err=tx.QueryRow(ctx,"SELECT installation_id FROM hub_installations WHERE token_hash=$1",hash).Scan(&existing)
 if err==nil{
  if existing!=id{fail(w,409,"Installation credential already assigned");return}
  if err=tx.Commit(ctx);err!=nil{fail(w,503,"Enrollment could not be saved");return}
  writeJSON(w,200,map[string]any{"registered":true});return
 }
 if !errors.Is(err,pgx.ErrNoRows){fail(w,503,"Tracking hub unavailable");return}
 if err=charge(ctx,tx,"all:enrollment",h.cfg.MaxInstallationsPerDay);err!=nil{fail(w,429,"New installation enrollment limit reached");return}
 _,err=tx.Exec(ctx,`INSERT INTO hub_installations(installation_id,token_hash) VALUES($1,$2)
 ON CONFLICT DO NOTHING`,id,hash)
 if err!=nil{fail(w,503,"Could not register installation");return}
 var confirmed string
 if err=tx.QueryRow(ctx,"SELECT token_hash FROM hub_installations WHERE installation_id=$1",id).Scan(&confirmed);err!=nil||confirmed!=hash{
  fail(w,409,"Installation ID is already registered");return
 }
 if err=tx.Commit(ctx);err!=nil{fail(w,503,"Could not save installation");return}
 writeJSON(w,201,map[string]any{"registered":true})
}
type trackRequest struct{
 ID string `json:"id"`
 Number string `json:"tracking_number"`
 Carrier string `json:"carrier"`
}
type trackingSnapshot struct{
 ID string `json:"id"`
 TrackingNumber string `json:"tracking_number"`
 Slug string `json:"slug"`
 Tag string `json:"tag"`
 SubtagMessage string `json:"subtag_message"`
 CourierEstimatedDeliveryDate json.RawMessage `json:"courier_estimated_delivery_date,omitempty"`
 AftershipEstimatedDeliveryDate json.RawMessage `json:"aftership_estimated_delivery_date,omitempty"`
 Checkpoints json.RawMessage `json:"checkpoints,omitempty"`
}
func(h *Hub) track(w http.ResponseWriter,r *http.Request){
 ctx,cancel:=context.WithTimeout(r.Context(),25*time.Second);defer cancel()
 installation,ok:=h.validInstallation(ctx,r);if !ok{fail(w,401,"Installation not enrolled");return}
 var in trackRequest
 if !decodeJSON(w,r,&in){return}
 id,err:=uuid.Parse(in.ID)
 in.Number=strings.ToUpper(strings.TrimSpace(in.Number))
 in.Carrier=strings.ToLower(strings.TrimSpace(in.Carrier))
 if err!=nil||id==uuid.Nil||!trackingNumberRE.MatchString(in.Number){
  fail(w,400,"Invalid shipment");return
 }
 switch in.Carrier {case "ups","usps","fedex","amazon","other":default:fail(w,400,"Unsupported carrier");return}
 tx,err:=h.cfg.DB.Begin(ctx);if err!=nil{fail(w,503,"Tracking hub unavailable");return}
 defer tx.Rollback(ctx)
 // Serializing per-installation requests prevents concurrent retries bypassing
 // shipment limits. We also recheck active credentials under the row lock.
 var active bool
 err=tx.QueryRow(ctx,"SELECT enabled FROM hub_installations WHERE installation_id=$1 FOR UPDATE",installation).Scan(&active)
 if err!=nil||!active{fail(w,403,"Installation disabled");return}
 _,err=tx.Exec(ctx,`INSERT INTO hub_shipments(installation_id,shipment_id,tracking_number,carrier)
 VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`,installation,id,in.Number,in.Carrier)
 if err!=nil{fail(w,503,"Could not register shipment");return}
 var count int
 if err=tx.QueryRow(ctx,"SELECT COUNT(*) FROM hub_shipments WHERE installation_id=$1",installation).Scan(&count);err!=nil{
  fail(w,503,"Could not check shipment quota");return
 }
 if count>h.cfg.MaxShipmentsPerInstall{fail(w,429,"Installation shipment quota reached");return}
 var savedNum,savedCarrier,providerID string
 var cached []byte
 var next time.Time
 err=tx.QueryRow(ctx,`SELECT tracking_number,carrier,provider_id,provider_data,next_fetch_at
 FROM hub_shipments WHERE installation_id=$1 AND shipment_id=$2 FOR UPDATE`,installation,id).
 Scan(&savedNum,&savedCarrier,&providerID,&cached,&next)
 if err!=nil{fail(w,503,"Could not load shipment");return}
 if savedNum!=in.Number||savedCarrier!=in.Carrier{
  // A shipment edit must be explicitly registered as a new provider identity,
  // and still remains scoped to this same installation.
  providerID="";cached=nil;next=time.Time{}
  _,err=tx.Exec(ctx,`UPDATE hub_shipments SET tracking_number=$3,carrier=$4,provider_id='',
    provider_data=NULL,next_fetch_at=now() WHERE installation_id=$1 AND shipment_id=$2`,
    installation,id,in.Number,in.Carrier)
  if err!=nil{fail(w,503,"Could not update shipment");return}
 }
 if len(cached)>0&&time.Now().Before(next){
  if err=tx.Commit(ctx);err!=nil{fail(w,503,"Tracking cache unavailable");return}
  var snap trackingSnapshot
  if json.Unmarshal(cached,&snap)!=nil{fail(w,503,"Tracking cache corrupted");return}
  writeJSON(w,200,map[string]any{"tracking":snap,"cached":true});return
 }
 if err=charge(ctx,tx,"install:"+installation.String(),h.cfg.MaxCallsPerInstallPerDay);err!=nil{
  fail(w,429,"Installation tracking quota reached");return
 }
 if err=charge(ctx,tx,"all:tracking",h.cfg.MaxGlobalCallsPerDay);err!=nil{
  fail(w,429,"Shared tracking service is at its daily limit");return
 }
 snapshot,err:=h.requestAfterShip(ctx,providerID,in.Number,in.Carrier)
 if err!=nil{
  // Charge the lookup attempt, but keep the shipment eligible for the next
  // periodic poll. Never return a fabricated status or provider secret.
  _=tx.Commit(ctx)
  fail(w,502,err.Error());return
 }
 raw,err:=json.Marshal(snapshot)
 if err!=nil{fail(w,502,"Tracking response invalid");return}
 _,err=tx.Exec(ctx,`UPDATE hub_shipments SET provider_id=$3,provider_data=$4,
 fetched_at=now(),next_fetch_at=now()+interval '3 hours',updated_at=now()
 WHERE installation_id=$1 AND shipment_id=$2`,installation,id,snapshot.ID,raw)
 if err!=nil{fail(w,503,"Could not save carrier update");return}
 if err=tx.Commit(ctx);err!=nil{fail(w,503,"Tracking update could not be saved");return}
 writeJSON(w,200,map[string]any{"tracking":snapshot,"cached":false})
}
func(h *Hub) sendProviderRequest(ctx context.Context,req *http.Request)(*http.Response,error){
 h.providerMu.Lock()
 defer h.providerMu.Unlock()
 if delay:=time.Until(h.nextProviderCall);delay>0{
  timer:=time.NewTimer(delay);defer timer.Stop()
  select{case <-ctx.Done():return nil,ctx.Err();case <-timer.C:}
 }
 h.nextProviderCall=time.Now().Add(210*time.Millisecond)
 return h.cfg.HTTPClient.Do(req)
}
func(h *Hub) requestAfterShip(ctx context.Context,providerID,number,carrier string)(trackingSnapshot,error){
 method,path:=http.MethodPost,h.cfg.Endpoint
 var input io.Reader
 if providerID!=""{
  if !safeIDRE.MatchString(providerID){return trackingSnapshot{},errors.New("Invalid provider tracking reference")}
  method=http.MethodGet;path+="/"+providerID
 }else{
  record:=map[string]string{"tracking_number":number}
  if carrier!="other"&&carrier!="fedex"{record["slug"]=carrier}
  bytesOut,_:=json.Marshal(record);input=bytes.NewReader(bytesOut)
 }
 req,err:=http.NewRequestWithContext(ctx,method,path,input)
 if err!=nil{return trackingSnapshot{},errors.New("Cannot prepare carrier request")}
 req.Header.Set("as-api-key",h.cfg.AfterShipKey)
 req.Header.Set("Content-Type","application/json")
 req.Header.Set("Accept","application/json")
 resp,err:=h.sendProviderRequest(ctx,req)
 if err!=nil{return trackingSnapshot{},errors.New("Carrier data service unreachable")}
 defer resp.Body.Close()
 if resp.StatusCode==401||resp.StatusCode==403{return trackingSnapshot{},errors.New("Shared carrier account not authorized")}
 if method==http.MethodPost&&resp.StatusCode==400{
  var problem struct{Meta struct{Code int `json:"code"`} `json:"meta"`}
  _=json.NewDecoder(io.LimitReader(resp.Body,4096)).Decode(&problem)
  if problem.Meta.Code==4003{
   // Existing tracking records belong to the same authorized AfterShip
   // account. Select ONLY this exact carrier/tracking number and return
   // minimal carrier status fields; never expose the account-wide list.
   return h.findExistingTracking(ctx,number,carrier)
  }
 }
 if resp.StatusCode==429{return trackingSnapshot{},errors.New("Carrier tracking rate limit reached")}
 if resp.StatusCode<200||resp.StatusCode>=300{return trackingSnapshot{},fmt.Errorf("Carrier tracking service HTTP %d",resp.StatusCode)}
 var envelope struct{Data trackingSnapshot `json:"data"`}
 if err=json.NewDecoder(io.LimitReader(resp.Body,2<<20)).Decode(&envelope);err!=nil{return trackingSnapshot{},errors.New("Carrier response unreadable")}
 out:=envelope.Data
 if !safeIDRE.MatchString(out.ID)||strings.ToUpper(strings.TrimSpace(out.TrackingNumber))!=number{
  return trackingSnapshot{},errors.New("Carrier response does not match shipment")
 }
 if len(out.Checkpoints)>100000||len(out.CourierEstimatedDeliveryDate)>5000||len(out.AftershipEstimatedDeliveryDate)>5000{
  return trackingSnapshot{},errors.New("Carrier response too large")
 }
 return out,nil
}
func(h *Hub) findExistingTracking(ctx context.Context,number,carrier string)(trackingSnapshot,error){
 endpoint:=h.cfg.Endpoint+"?tracking_numbers="+url.QueryEscape(number)+"&limit=50"
 req,err:=http.NewRequestWithContext(ctx,http.MethodGet,endpoint,nil)
 if err!=nil{return trackingSnapshot{},errors.New("Cannot locate existing tracking")}
 req.Header.Set("as-api-key",h.cfg.AfterShipKey)
 req.Header.Set("Accept","application/json")
 resp,err:=h.sendProviderRequest(ctx,req)
 if err!=nil{return trackingSnapshot{},errors.New("Carrier record lookup failed")}
 defer resp.Body.Close()
 if resp.StatusCode!=200{return trackingSnapshot{},errors.New("Carrier did not return matching tracking record")}
 var list struct{Data struct{Trackings []trackingSnapshot `json:"trackings"`} `json:"data"`}
 if json.NewDecoder(io.LimitReader(resp.Body,2<<20)).Decode(&list)!=nil{return trackingSnapshot{},errors.New("Carrier duplicate lookup unreadable")}
 for _,item:=range list.Data.Trackings{
  if strings.ToUpper(strings.TrimSpace(item.TrackingNumber))!=number||!safeIDRE.MatchString(item.ID){continue}
  slug:=strings.ToLower(item.Slug)
  if carrier!="other"&&slug!=carrier&&!strings.HasPrefix(slug,carrier+"-"){continue}
  // Fetch by provider tracking ID to retrieve full checkpoint history.
  return h.requestAfterShip(ctx,item.ID,number,carrier)
 }
 return trackingSnapshot{},errors.New("Carrier has no matching tracking record for this number and courier")
}
func (h *Hub) Prune(ctx context.Context)error{
 _,err:=h.cfg.DB.Exec(ctx,`DELETE FROM hub_shipments WHERE updated_at<now()-interval '45 days'`)
 if err!=nil{return err}
 _,err=h.cfg.DB.Exec(ctx,`DELETE FROM hub_usage WHERE usage_day<CURRENT_DATE-7`)
 return err
}
func RemoteClientIP(r *http.Request)string{
 addr,_,err:=net.SplitHostPort(r.RemoteAddr)
 if err!=nil{return "unknown"}
 return addr
}
