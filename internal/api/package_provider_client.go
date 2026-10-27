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
 "regexp"
 "strings"
 "time"

 "github.com/google/uuid"
)

const aftershipEndpoint = "https://api.aftership.com/tracking/2026-07/trackings"

var (
 upsTrackingRE=regexp.MustCompile(`^1Z[0-9A-Z]{16}$`)
 amazonTrackingRE=regexp.MustCompile(`^(?:TBA|TBM|TBC)[0-9]{12,15}$`)
 uspsTrackingRE=regexp.MustCompile(`^(?:9[2345][0-9]{18,22}|[A-Z]{2}[0-9]{9}US)$`)
 fedexTrackingRE=regexp.MustCompile(`^(?:[0-9]{12}|[0-9]{15})$`)
 providerIDRE=regexp.MustCompile(`^[a-zA-Z0-9_-]{1,128}$`)
)

func normalizePackageTrackingNumber(number string) string {
 return strings.Map(func(r rune)rune{if r=='-'||r==' '||r==9||r==10||r==13{return -1};return r},strings.ToUpper(strings.TrimSpace(number)))
}
func detectPackageCarrier(number string) string {
 number=normalizePackageTrackingNumber(number)
 switch {
 case upsTrackingRE.MatchString(number):return "ups"
 case amazonTrackingRE.MatchString(number):return "amazon"
 case uspsTrackingRE.MatchString(number):return "usps"
 case fedexTrackingRE.MatchString(number):return "fedex"
 default:return "other"
 }
}
func providerCourier(slug string) string {
 switch {
 case slug=="ups"||strings.HasPrefix(slug,"ups-"):return "ups"
 case slug=="usps"||strings.HasPrefix(slug,"usps-"):return "usps"
 case slug=="fedex"||strings.HasPrefix(slug,"fedex-"):return "fedex"
 case slug=="amazon"||strings.HasPrefix(slug,"amazon-"):return "amazon"
 default:return "other"
 }
}
func providerStatus(tag string) string {
 switch strings.ToLower(strings.TrimSpace(tag)){
 case "inforeceived":return "label_created"
 case "intransit","availableforpickup":return "in_transit"
 case "outfordelivery":return "out_for_delivery"
 case "delivered":return "delivered"
 case "exception","attemptfail","expired":return "exception"
 default:return ""
 }
}
func providerDate(value string) string{
 if len(value)<10{return ""}
 date:=value[:10]
 d,err:=time.Parse("2006-01-02",date)
 if err!=nil||d.Format("2006-01-02")!=date{return ""}
 return date
}

type aftershipCheckpoint struct {
 Hash string `json:"hash"`
 Tag string `json:"tag"`
 Message string `json:"message"`
 CheckpointTime string `json:"checkpoint_time"`
 CreatedAt string `json:"created_at"`
 Source string `json:"source"`
}
type aftershipDeliveryDate struct {
 EstimatedDeliveryDate string `json:"estimated_delivery_date"`
}
type aftershipTracking struct {
 ID string `json:"id"`
 Slug string `json:"slug"`
 Tag string `json:"tag"`
 TrackingNumber string `json:"tracking_number"`
 SubtagMessage string `json:"subtag_message"`
 CourierEstimatedDeliveryDate *aftershipDeliveryDate `json:"courier_estimated_delivery_date"`
 AftershipEstimatedDeliveryDate *aftershipDeliveryDate `json:"aftership_estimated_delivery_date"`
 Checkpoints []aftershipCheckpoint `json:"checkpoints"`
}
func (t aftershipTracking) deliveryDate()string {
 if t.CourierEstimatedDeliveryDate!=nil {
  if d:=providerDate(t.CourierEstimatedDeliveryDate.EstimatedDeliveryDate);d!=""{return d}
 }
 if t.AftershipEstimatedDeliveryDate!=nil{return providerDate(t.AftershipEstimatedDeliveryDate.EstimatedDeliveryDate)}
 return ""
}
func (p aftershipCheckpoint) reference()string {
 if p.Hash!=""&&len(p.Hash)<160{return "aftership:"+p.Hash}
 data:=p.CheckpointTime+"|"+p.CreatedAt+"|"+p.Tag+"|"+p.Message
 hash:=sha256.Sum256([]byte(data))
 return "aftership:"+hex.EncodeToString(hash[:])
}
func (p aftershipCheckpoint) occurredAt()time.Time {
 for _,value:=range []string{p.CheckpointTime,p.CreatedAt}{
  for _,layout:=range []string{time.RFC3339Nano,"2006-01-02T15:04:05Z0700"}{
   t,err:=time.Parse(layout,value)
   if err==nil{return t}
  }
 }
 return time.Now().UTC()
}
type aftershipAPI struct {
 key string
 baseURL string
 client *http.Client
}
func newAftershipAPI(key string)*aftershipAPI{
 return &aftershipAPI{key:key,baseURL:aftershipEndpoint,client:&http.Client{Timeout:12*time.Second,CheckRedirect:func(*http.Request,[]*http.Request)error{return http.ErrUseLastResponse}}}
}
func(a *aftershipAPI) request(ctx context.Context,method,path string,body any)(aftershipTracking,error){
 var b io.Reader
 if body!=nil{
  raw,err:=json.Marshal(body);if err!=nil{return aftershipTracking{},err}
  b=bytes.NewReader(raw)
 }
 req,err:=http.NewRequestWithContext(ctx,method,a.baseURL+path,b)
 if err!=nil{return aftershipTracking{},err}
 req.Header.Set("as-api-key",a.key)
 req.Header.Set("Accept","application/json")
 req.Header.Set("Content-Type","application/json")
 res,err:=a.client.Do(req)
 if err!=nil{return aftershipTracking{},errors.New("Tracking provider connection failed")}
 defer res.Body.Close()
 if res.StatusCode<200||res.StatusCode>=300{
  if res.StatusCode==401||res.StatusCode==403{return aftershipTracking{},errors.New("Tracking API credentials or plan are not authorized")}
  if res.StatusCode==429{return aftershipTracking{},errors.New("Tracking provider rate limit; next attempt in three hours")}
  if res.StatusCode==409{return aftershipTracking{},errors.New("Tracking already registered with provider")}
  return aftershipTracking{},fmt.Errorf("Tracking provider returned HTTP %d",res.StatusCode)
 }
 var envelope struct {
  Meta struct{Code int `json:"code"`} `json:"meta"`
  Data aftershipTracking `json:"data"`
 }
 if err=json.NewDecoder(io.LimitReader(res.Body,2<<20)).Decode(&envelope);err!=nil{return aftershipTracking{},errors.New("Invalid tracking provider response")}
 if envelope.Data.ID==""||!providerIDRE.MatchString(envelope.Data.ID){return aftershipTracking{},errors.New("Tracking provider did not return a valid tracking ID")}
 return envelope.Data,nil
}
func(a *aftershipAPI) fetch(ctx context.Context,id string)(aftershipTracking,error){
 if !providerIDRE.MatchString(id){return aftershipTracking{},errors.New("Invalid saved provider reference")}
 return a.request(ctx,http.MethodGet,"/"+id,nil)
}
func(a *aftershipAPI) register(ctx context.Context,id uuid.UUID,number,carrier string)(aftershipTracking,error){
 trackingID:="calden-"+strings.ReplaceAll(id.String(),"-","")
 record:=map[string]any{"id":trackingID,"tracking_number":normalizePackageTrackingNumber(number)}
 // Unknown or guessed ambiguous carrier is intentionally omitted: AfterShip
 // identifies the courier using its enabled preferred courier list.
 if carrier=="ups"||carrier=="usps"||carrier=="amazon"{
  record["slug"]=carrier
 }
 result,err:=a.request(ctx,http.MethodPost,"",record)
 if err!=nil&&strings.Contains(err.Error(),"already registered"){return a.fetch(ctx,trackingID)}
 return result,err
}
