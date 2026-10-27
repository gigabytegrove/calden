package api

import (
 "bytes"
 "context"
 "crypto/hmac"
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "errors"
 "io"
 "net/http"
 "net/url"
 "os"
 "strings"
 "time"

 "github.com/google/uuid"
)

type hubTrackingResponse struct {
 Tracking aftershipTracking `json:"tracking"`
 Cached bool `json:"cached"`
}
func trackingHubURL() (string,error){
 raw:=strings.TrimSpace(os.Getenv("CALDEN_TRACKING_HUB_URL"))
 if raw==""{return "",nil}
 u,err:=url.Parse(raw)
 if err!=nil||u.Scheme!="https"||u.Hostname()==""||u.User!=nil||u.RawQuery!=""||u.Fragment!=""||u.Path!=""&&u.Path!="/"{
  return "",errors.New("Calden Tracking Hub URL must be an HTTPS origin")
 }
 return strings.TrimSuffix(raw,"/"),nil
}
func hubCredential(secret []byte)(uuid.UUID,string,error){
 if len(secret)<32{return uuid.Nil,"",errors.New("Calden persistent secret unavailable")}
 mac:=hmac.New(sha256.New,secret)
 _,_=mac.Write([]byte("calden/tracking-hub/install-v1"))
 token:=hex.EncodeToString(mac.Sum(nil))
 id:=uuid.NewSHA1(uuid.NameSpaceOID,[]byte("calden-tracking-install:"+token))
 return id,token,nil
}
func hubClient()*http.Client{
 return &http.Client{
  Timeout:18*time.Second,
  CheckRedirect:func(*http.Request,[]*http.Request)error{return http.ErrUseLastResponse},
 }
}
func hubRequest(ctx context.Context,client *http.Client,origin,credential,path string,body any,response any)error{
 bytesOut,err:=json.Marshal(body)
 if err!=nil{return errors.New("Could not encode tracking request")}
 req,err:=http.NewRequestWithContext(ctx,http.MethodPost,origin+path,bytes.NewReader(bytesOut))
 if err!=nil{return errors.New("Could not prepare tracking service request")}
 req.Header.Set("Authorization","Bearer "+credential)
 req.Header.Set("Content-Type","application/json")
 req.Header.Set("Accept","application/json")
 res,err:=client.Do(req)
 if err!=nil{return errors.New("Shared tracking service is not reachable")}
 defer res.Body.Close()
 if res.StatusCode<200||res.StatusCode>=300{
  var failure struct{Error string `json:"error"`}
  _=json.NewDecoder(io.LimitReader(res.Body,4096)).Decode(&failure)
  if res.StatusCode==429{return errors.New("Shared tracking service quota reached; next attempt will be automatic")}
  if res.StatusCode==403{return errors.New("Shared tracking service is not accepting new installations")}
  if res.StatusCode==401{return errors.New("Shared tracking installation authorization failed")}
  if res.StatusCode==502{
   if len(failure.Error)>0&&len(failure.Error)<200{return errors.New(failure.Error)}
   return errors.New("Carrier tracking service was unable to retrieve this shipment")
  }
  return errors.New("Shared tracking service could not complete lookup")
 }
 if err=json.NewDecoder(io.LimitReader(res.Body,1<<20)).Decode(response);err!=nil{return errors.New("Shared tracking response is invalid")}
 return nil
}
func fetchHubTracking(ctx context.Context,client *http.Client,origin string,secret []byte,job dueTracking)(aftershipTracking,error){
 installation,credential,err:=hubCredential(secret)
 if err!=nil{return aftershipTracking{},err}
 var enrolled struct{Registered bool `json:"registered"`}
 err=hubRequest(ctx,client,origin,credential,"/v1/installations",map[string]string{"installation_id":installation.String()},&enrolled)
 if err!=nil{return aftershipTracking{},err}
 if !enrolled.Registered{return aftershipTracking{},errors.New("Shared tracking installation enrollment failed")}
 var response hubTrackingResponse
 err=hubRequest(ctx,client,origin,credential,"/v1/trackings",map[string]string{
  "id":job.ID.String(),"tracking_number":job.Number,"carrier":job.Carrier,
 },&response)
 if err!=nil{return aftershipTracking{},err}
 if response.Tracking.ID==""||normalizePackageTrackingNumber(response.Tracking.TrackingNumber)!=job.Number{
  return aftershipTracking{},errors.New("Shared tracking returned a different shipment")
 }
 return response.Tracking,nil
}
