package api

import (
 "bytes"
 "context"
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "net/http"
 "os"
 "os/exec"
 "path/filepath"
 "regexp"
 "strings"
 "time"
)

var errLocalScraperUnavailable=errors.New("bundled local tracking engine not available")
var localScraperNumberRE=regexp.MustCompile(`^[A-Z0-9]{4,100}$`)

type localScan struct {
 Time string `json:"time"`
 Instant string `json:"instant"`
 LocalTime string `json:"local_time"`
 Location string `json:"location"`
 Description string `json:"description"`
 Stage string `json:"stage"`
}
type localResult struct {
 Carrier string `json:"carrier"`
 Source string `json:"source"`
 Result struct {
  CurrentStage string `json:"current_stage"`
  Status string `json:"status"`
  Events []localScan `json:"events"`
  ExpectedDelivery string `json:"expected_delivery"`
  DeliveryTrackingNumber string `json:"delivery_tracking_number"`
  CanonicalTrackingNumber string `json:"canonical_tracking_number"`
 } `json:"result"`
}
func normalizeLocalStage(value string) string {
 value=strings.ToLower(strings.TrimSpace(value))
 value=strings.NewReplacer("-","_"," ","_").Replace(value)
 switch value {
 case "delivered":return "delivered"
 case "out_for_delivery","outfordelivery":return "out_for_delivery"
 case "in_transit","intransit","in_transit_to_destination","available_for_pickup","ready_for_pickup","in_customs","customs_clearance","customs":return "in_transit"
 case "label_created","info_received","pre_transit","not_yet_shipped","accepted","registered":return "label_created"
 case "exception","failed_attempt","delivery_failed","returned","return_to_sender","lost":return "exception"
 default:return ""
 }
}
func normalizeLocalCarrier(raw string)string{
 raw=strings.ToLower(strings.TrimSpace(raw))
 switch{
 case raw=="ups"||strings.HasPrefix(raw,"ups-"):return "ups"
 case raw=="usps"||strings.HasPrefix(raw,"usps-"):return "usps"
 case raw=="fedex"||strings.HasPrefix(raw,"fedex-"):return "fedex"
 case raw=="amazon"||strings.HasPrefix(raw,"amazon-"):return "amazon"
 default:return "other"
 }
}
func parseLocalScraperJSON(raw []byte,number string)(localResult,error){
 var response localResult
 if len(raw)==0||len(raw)>2<<20{return response,errors.New("Tracking response exceeded safe size")}
 if err:=json.Unmarshal(raw,&response);err!=nil{return response,errors.New("Local tracker returned invalid JSON")}
 if response.Result.CurrentStage==""&&response.Result.Status==""&&len(response.Result.Events)==0{return response,errors.New("Carrier supplied no shipment events")}
 if len(response.Result.Events)>250{return response,errors.New("Carrier history exceeded safe event limit")}
 if len(number)<4||!localScraperNumberRE.MatchString(number){return response,errors.New("Invalid tracking number")}
 // The source may provide a corrected/canonical number during an official
 // handoff. Its primary delivery number, however, must never silently
 // update a different household shipment.
 if reported:=normalizePackageTrackingNumber(response.Result.DeliveryTrackingNumber);reported!=""&&reported!=number{
  if canonical:=normalizePackageTrackingNumber(response.Result.CanonicalTrackingNumber);canonical!=number{
   return response,errors.New("Carrier response describes a different tracking number")
  }
 }
 return response,nil
}
func localScannerExecutable()string{
 // Docker includes the engine in its own image; no separate service.
 if custom:=strings.TrimSpace(os.Getenv("CALDEN_LOCAL_PARCEL_CLI"));custom!=""{return custom}
 if parcelManagedSelected()&&parcelExists(parcelBundledCLI()){return parcelBundledCLI()}
 if parcelExists("/opt/calden-parcel/node_modules/universal-parcel-scraper/dist/cli/index.js"){return "/opt/calden-parcel/node_modules/universal-parcel-scraper/dist/cli/index.js"}
 return parcelBundledCLI()
}
// trackerRuntimeReadiness performs only offline checks and never sends a
// household tracking number to an external website.
func trackerRuntimeReadiness(ctx context.Context)(bool,bool,string){
 path:=localScannerExecutable()
 info,err:=os.Stat(path)
 if err!=nil||!info.Mode().IsRegular(){
  _,message:=parcelInstallSnapshot()
  return false,false,message
 }
 node:="node"
 // Do not accidentally execute the healthy embedded CLI with a stale Node
 // binary left in the managed directory after a failed earlier repair.
 if path==parcelBundledCLI(){node=parcelBundledNode()}
 if _,err:=exec.LookPath(node);err!=nil{return false,false,"Calden is automatically preparing tracking."}
 chromium:=false
 if info,err:=os.Stat("/usr/bin/chromium");err==nil&&!info.IsDir()&&info.Mode()&0111!=0{chromium=true}
 if parcelExists(parcelBundledChromium()){chromium=true}
 probeCtx,cancel:=context.WithTimeout(ctx,6*time.Second)
 defer cancel()
 cmd:=exec.CommandContext(probeCtx,node,path,"detect","1Z999AA10123456784")
 cmd.Env=[]string{"PATH="+os.Getenv("PATH"),"LANG=C.UTF-8","HOME="+os.Getenv("HOME"),"NODE_OPTIONS=--max-old-space-size=512"}
 if node==parcelBundledNode(){cmd.Env=parcelRuntimeEnv(filepath.Join(parcelStoreRoot(),"current"))}
 cmd.Stderr=io.Discard
 out:=&limitedTrackingBuffer{cap:128<<10}
 cmd.Stdout=out
 if cmd.Run()!=nil||!strings.Contains(strings.ToLower(out.String()),"ups"){
  return false,chromium,"The bundled tracking engine failed its offline startup test."
 }
 if !chromium{return false,false,"Calden is repairing the browser required for tracking."}
 return true,true,"The bundled tracking engine and Chromium are ready. Carrier access varies by provider."
}
func (s *server) packageTrackerHealth(w http.ResponseWriter,r *http.Request){
 if !s.requirePackages(w,r){return}
 if currentActor(r).IsChild{writeError(w,http.StatusForbidden,"Package tracking is unavailable to child accounts");return}
 state,detail:=parcelInstallSnapshot()
 if state=="installing"||state=="retrying"||state=="pending"{
  // A download can take time. The UI should show background recovery, not
  // probe half-extracted binaries or present a carrier lookup error.
  writeJSON(w,http.StatusOK,map[string]any{"ready":false,"browser_ready":false,"state":state,"message":detail})
  return
 }
 ready,browser,message:=trackerRuntimeReadiness(r.Context())
 if ready{state="ready"}
 writeJSON(w,http.StatusOK,map[string]any{"ready":ready,"browser_ready":browser,"state":state,"message":message})
}
func invokeLocalScraper(ctx context.Context,number,carrier string)(localResult,error){
 var empty localResult
 if !localScraperNumberRE.MatchString(number){return empty,errors.New("Invalid tracking number")}
 path:=localScannerExecutable()
 if _,err:=os.Stat(path);err!=nil{return empty,errLocalScraperUnavailable}
 args:=[]string{path,"track",number}
 // Amazon TBA identifiers require a provider that supports them, and
 // generic FedEx numeric identifiers overlap other carriers. Ask the
 // engine to recognize candidates rather than forcing an unverified guess.
 if carrier=="ups"||carrier=="usps"{args=append(args,"--carrier",carrier)}
 node:="node"
 if path==parcelBundledCLI(){node=parcelBundledNode()}
 cmd:=exec.CommandContext(ctx,node,args...)
 // Do not hand Calden database passwords, JWT secrets, SMTP credentials, or
 // other service configuration to an external tracking adapter subprocess.
 // Explicitly allowlist its environment instead of inheriting os.Environ().
 cmd.Env=[]string{
  "PATH="+os.Getenv("PATH"),
  "HOME="+os.Getenv("HOME"),
  "LANG=C.UTF-8",
  "NODE_OPTIONS=--max-old-space-size=512",
  "TRACKING_CHROMIUM_PATH=/usr/bin/chromium",
  "SCRAPER_PROVIDERS=ParcelsApp,Ship24,17TRACK,Postal Ninja,UPU",
 }
 if path==parcelBundledCLI(){cmd.Env=parcelRuntimeEnv(filepath.Join(parcelStoreRoot(),"current"))}
 if agents:=strings.TrimSpace(os.Getenv("CALDEN_TRACKING_USER_AGENT"));agents!=""&&len(agents)<180&&!strings.ContainsAny(agents,"\\r\\n"){
  cmd.Env=append(cmd.Env,"SCRAPER_USER_AGENT="+agents)
 }
 cmd.WaitDelay=3*time.Second
 // Only stdout may contain structured data. Never store command stderr,
 // which may include provider internals or raw tracking details.
 out:=&limitedTrackingBuffer{cap:2<<20}
 cmd.Stdout=out
 cmd.Stderr=io.Discard
 if err:=cmd.Run();err!=nil{
  if ctx.Err()!=nil{return empty,errors.New("Carrier lookup timed out")}
  if errors.Is(err,exec.ErrNotFound){return empty,errLocalScraperUnavailable}
  return empty,errors.New("Carrier has not returned usable tracking history")
 }
 return parseLocalScraperJSON(out.Bytes(),number)
}
type limitedTrackingBuffer struct{
 bytes.Buffer
 cap int
}
func (b *limitedTrackingBuffer)Write(raw []byte)(int,error){
 if b.Len()+len(raw)>b.cap{return 0,errors.New("Tracking response too large")}
 return b.Buffer.Write(raw)
}
func localScanRef(scan localScan)string {
 h:=sha256.Sum256([]byte(strings.Join([]string{scan.Time,scan.Stage,scan.Location,scan.Description},"|")))
 return "local:"+hex.EncodeToString(h[:])
}
func localScanTime(value string)time.Time{
 // Only use absolute times with an explicit offset. A carrier's unzoned
 // local wall clock must not be silently treated as UTC.
 for _,format:=range []string{time.RFC3339Nano,time.RFC3339,"2006-01-02T15:04:05-0700","2006-01-02 15:04:05 -0700"}{
  if t,err:=time.Parse(format,value);err==nil{return t.UTC()}
 }
 return time.Time{}
}
// Keep carrier activity with unfamiliar stage codes instead of silently
// deleting it. Unknown carrier stages do not imply a delivery state.
func localScanCheckpoint(e localScan)(string,string,bool){
 code:=normalizeLocalStage(e.Stage)
 desc:=strings.TrimSpace(e.Description)
 if loc:=strings.TrimSpace(e.Location);loc!=""{desc+=" · "+loc}
 if code==""{
  if desc==""&&strings.TrimSpace(e.Stage)==""{return "","",false}
  code="unknown"
  if raw:=cleanText(strings.TrimSpace(e.Stage),64);raw!=""{desc="Carrier event ("+raw+"): "+desc}
 }
 desc=cleanText(desc,480)
 if desc==""{desc="Carrier reported "+e.Stage}
 return code,desc,true
}
func localCurrentStage(response localResult)string{
 if stage:=normalizeLocalStage(response.Result.CurrentStage);stage!=""{return stage}
 if stage:=normalizeLocalStage(response.Result.Status);stage!=""{return stage}
 var newest time.Time
 stage:=""
 // A carrier's events may be oldest-first or unordered.
 for _,e:=range response.Result.Events{
  mapped:=normalizeLocalStage(e.Stage)
  if mapped==""{continue}
  at:=localScanTime(e.Instant)
  if at.IsZero(){at=localScanTime(e.Time)}
  if !at.IsZero()&&at.After(newest){newest=at;stage=mapped}
 }
 return stage
}
func(s *server) syncLocalScraperTracking(ctx context.Context,job dueTracking)error{
 response,err:=invokeLocalScraper(ctx,job.Number,job.Carrier)
 if err!=nil{return err}
 stage:=localCurrentStage(response)
 if stage==""&&len(response.Result.Events)==0{return errors.New("Carrier supplied no usable shipment activity")}
 carrier:=normalizeLocalCarrier(response.Carrier)
 if carrier=="other"{carrier=job.Carrier}
 date:=providerDate(response.Result.ExpectedDelivery)
 tx,err:=s.db.Begin(ctx);if err!=nil{return err}
 defer tx.Rollback(ctx)
 label:="local:"+strings.ToLower(strings.TrimSpace(response.Source))
 if len(label)>128{label=label[:128]}
 tag,err:=tx.Exec(ctx,`UPDATE package_shipments SET
 status=CASE WHEN $3='' THEN status ELSE $3 END,
 status_source=CASE WHEN $3='' THEN status_source ELSE 'provider' END,
 carrier=$4,provider_slug=$5,
 expected_date=COALESCE(NULLIF($6,'')::date,expected_date),
 delivered_at=CASE WHEN $3='delivered' THEN COALESCE(delivered_at,now()) ELSE delivered_at END,
 last_tracking_check_at=now(),tracking_error='',updated_at=now()
 WHERE id=$1 AND tracking_number=$2 AND NOT archived`,
 job.ID,job.Number,stage,carrier,label,date)
 if err!=nil{return err}
 if tag.RowsAffected()!=1{return errors.New("Shipment changed before carrier update completed")}
 recorded:=0
 for _,e:=range response.Result.Events{
  code,desc,ok:=localScanCheckpoint(e);if !ok{continue}
  eventAt:=localScanTime(e.Instant)
  if eventAt.IsZero(){eventAt=localScanTime(e.Time)}
  // A human-visible local timestamp is useful but must not be converted
  // into an invented UTC instant when the timezone is unknown.
  if eventAt.IsZero()&&e.LocalTime!=""{desc+=" ("+strings.TrimSpace(e.LocalTime)+" local time)"}
  if eventAt.IsZero(){
   desc=cleanText(desc+" (carrier did not provide a verified scan time)",480)
   eventAt=time.Now().UTC()
  }
  _,err=tx.Exec(ctx,`INSERT INTO package_checkpoints(shipment_id,status,detail,source,occurred_at,provider_ref)
  VALUES($1,$2,$3,'provider',$4,$5) ON CONFLICT DO NOTHING`,
  job.ID,code,desc,eventAt,localScanRef(e))
  if err!=nil{return fmt.Errorf("Could not record carrier event: %w",err)}
  recorded++
 }
 if recorded==0&&stage!=""{
  ref:="local:current:"+stage
  _,err=tx.Exec(ctx,`INSERT INTO package_checkpoints(shipment_id,status,detail,source,provider_ref)
  VALUES($1,$2,'Carrier status reported (no scan supplied)','provider_status',$3) ON CONFLICT DO NOTHING`,job.ID,stage,ref)
  if err!=nil{return err}
 }
 if recorded==0&&stage==""{return errors.New("Carrier did not supply usable scan details")}
 return tx.Commit(ctx)
}
