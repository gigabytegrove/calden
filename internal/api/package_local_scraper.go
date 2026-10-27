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
 "os"
 "os/exec"
 "regexp"
 "strings"
 "time"
)

var errLocalScraperUnavailable=errors.New("bundled local tracking engine not available")
var localScraperNumberRE=regexp.MustCompile(`^[A-Z0-9]{4,100}$`)

type localScan struct {
 Time string `json:"time"`
 Location string `json:"location"`
 Description string `json:"description"`
 Stage string `json:"stage"`
}
type localResult struct {
 Carrier string `json:"carrier"`
 Source string `json:"source"`
 Result struct {
  CurrentStage string `json:"current_stage"`
  Events []localScan `json:"events"`
  ExpectedDelivery string `json:"expected_delivery"`
 } `json:"result"`
}
func normalizeLocalStage(value string) string {
 value=strings.ToLower(strings.TrimSpace(value))
 switch value {
 case "delivered":return "delivered"
 case "out_for_delivery":return "out_for_delivery"
 case "in_transit","in_transit_to_destination","available_for_pickup","in_customs","customs_clearance":return "in_transit"
 case "label_created","info_received","pre_transit","not_yet_shipped","accepted":return "label_created"
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
 if response.Result.CurrentStage==""&&len(response.Result.Events)==0{return response,errors.New("Carrier supplied no shipment events")}
 if len(response.Result.Events)>250{return response,errors.New("Carrier history exceeded safe event limit")}
 if len(number)<4||!localScraperNumberRE.MatchString(number){return response,errors.New("Invalid tracking number")}
 return response,nil
}
func localScannerExecutable()string{
 // Docker includes the engine in its own image; no separate service.
 if custom:=strings.TrimSpace(os.Getenv("CALDEN_LOCAL_PARCEL_CLI"));custom!=""{return custom}
 return "/opt/calden-parcel/node_modules/universal-parcel-scraper/dist/cli/index.js"
}
func invokeLocalScraper(ctx context.Context,number,carrier string)(localResult,error){
 var empty localResult
 if !localScraperNumberRE.MatchString(number){return empty,errors.New("Invalid tracking number")}
 path:=localScannerExecutable()
 if _,err:=os.Stat(path);err!=nil{return empty,errLocalScraperUnavailable}
 args:=[]string{path,"track",number}
 if carrier!="other"&&carrier!=""{args=append(args,"--carrier",carrier)}
 cmd:=exec.CommandContext(ctx,"node",args...)
 cmd.Env=append(os.Environ(),"TRACKING_CHROMIUM_PATH=/usr/bin/chromium")
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
 for _,format:=range []string{time.RFC3339Nano,time.RFC3339,"2006-01-02 15:04:05","2006-01-02T15:04:05","2006-01-02"}{
  if t,err:=time.Parse(format,value);err==nil{return t.UTC()}
 }
 return time.Now().UTC()
}
func(s *server) syncLocalScraperTracking(ctx context.Context,job dueTracking)error{
 response,err:=invokeLocalScraper(ctx,job.Number,job.Carrier)
 if err!=nil{return err}
 stage:=normalizeLocalStage(response.Result.CurrentStage)
 if stage==""{
  for _,event:=range response.Result.Events{
   if mapped:=normalizeLocalStage(event.Stage);mapped!=""{stage=mapped;break}
  }
 }
 if stage==""{return errors.New("Carrier returned an unrecognized shipment state")}
 carrier:=normalizeLocalCarrier(response.Carrier)
 if carrier=="other"{carrier=job.Carrier}
 date:=providerDate(response.Result.ExpectedDelivery)
 tx,err:=s.db.Begin(ctx);if err!=nil{return err}
 defer tx.Rollback(ctx)
 label:="local:"+strings.ToLower(strings.TrimSpace(response.Source))
 if len(label)>128{label=label[:128]}
 tag,err:=tx.Exec(ctx,`UPDATE package_shipments SET
 status=$3,status_source='provider',carrier=$4,provider_slug=$5,
 expected_date=COALESCE(NULLIF($6,'')::date,expected_date),
 delivered_at=CASE WHEN $3='delivered' THEN COALESCE(delivered_at,now()) ELSE delivered_at END,
 last_tracking_check_at=now(),tracking_error='',updated_at=now()
 WHERE id=$1 AND tracking_number=$2 AND NOT archived`,
 job.ID,job.Number,stage,carrier,label,date)
 if err!=nil{return err}
 if tag.RowsAffected()!=1{return errors.New("Shipment changed before carrier update completed")}
 for _,e:=range response.Result.Events{
  code:=normalizeLocalStage(e.Stage);if code==""{continue}
  desc:=strings.TrimSpace(e.Description)
  if loc:=strings.TrimSpace(e.Location);loc!=""{desc+=" · "+loc}
  desc=cleanText(desc,480)
  if desc==""{desc="Carrier reported "+e.Stage}
  _,err=tx.Exec(ctx,`INSERT INTO package_checkpoints(shipment_id,status,detail,source,occurred_at,provider_ref)
  VALUES($1,$2,$3,'provider',$4,$5) ON CONFLICT DO NOTHING`,
  job.ID,code,desc,localScanTime(e.Time),localScanRef(e))
  if err!=nil{return fmt.Errorf("Could not record carrier event: %w",err)}
 }
 if len(response.Result.Events)==0{
  ref:="local:current:"+stage
  _,err=tx.Exec(ctx,`INSERT INTO package_checkpoints(shipment_id,status,detail,source,provider_ref)
  VALUES($1,$2,'Carrier status retrieved','provider',$3) ON CONFLICT DO NOTHING`,job.ID,stage,ref)
  if err!=nil{return err}
 }
 return tx.Commit(ctx)
}
