package api

import (
 "context"
 "fmt"
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"
 "time"

 "github.com/google/uuid"
)

func TestPackageCarrierDetection(t *testing.T){
 for _,tt:=range []struct{number,want string}{
  {"1Z999AA10123456784","ups"},
  {"tba123456789012","amazon"},
  {"9400111899223856922204","usps"},
  {"123456789012","fedex"},
  {"123456789012345","fedex"},
  {"UNRECOGNIZED123","other"},
  {"9999","other"},
 }{
  if got:=detectPackageCarrier(tt.number);got!=tt.want{t.Errorf("%q: got %q want %q",tt.number,got,tt.want)}
  input:=packageInput{Label:"Parcel",Carrier:"auto",TrackingNumber:tt.number,Status:"unknown"}
  if !preparePackageInput(&input)||input.Carrier!=tt.want{t.Errorf("auto carrier failed for %q: %#v",tt.number,input)}
 }
}
func TestPackageProviderMappingAndDates(t *testing.T){
 for _,tt:=range []struct{tag,status string}{
  {"Pending",""},{"InfoReceived","label_created"},{"InTransit","in_transit"},
  {"OutForDelivery","out_for_delivery"},{"Delivered","delivered"},
  {"AttemptFail","exception"},{"AvailableForPickup","in_transit"},{"Exception","exception"},
 }{if got:=providerStatus(tt.tag);got!=tt.status{t.Errorf("%s -> %s, want %s",tt.tag,got,tt.status)}}
 if providerDate("2026-02-30")!=""||providerDate("2026-11-02T15:05:00Z")!="2026-11-02"{t.Fatal("tracking ETA parser accepted invalid date or rejected timestamp")}
 if providerCourier("fedex-ground")!="fedex"||providerCourier("amazon-logistics")!="amazon"{t.Fatal("courier slug not normalized")}
}
func TestAftershipAPIUsesOnlyFixedMethodAndTrustedData(t *testing.T){
 var created,queried int
 ts:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  if r.Header.Get("as-api-key")!="test-secret"{t.Errorf("missing authorized key")}
  w.Header().Set("Content-Type","application/json")
  switch{
  case r.Method=="POST"&&r.URL.Path=="/trackings":
   created++
   fmt.Fprint(w,`{"meta":{"code":201},"data":{"id":"calden-00000000000000000000000000000001","slug":"usps","tag":"InTransit","tracking_number":"9400111899223856922204","courier_estimated_delivery_date":{"estimated_delivery_date":"2026-11-02"},"checkpoints":[{"hash":"evt123","tag":"InfoReceived","message":"Label created","source":"carrier","checkpoint_time":"2026-10-27T09:00:00Z"}]}}`)
  case r.Method=="GET"&&r.URL.Path=="/trackings/calden-00000000000000000000000000000001":
   queried++
   fmt.Fprint(w,`{"meta":{"code":200},"data":{"id":"calden-00000000000000000000000000000001","slug":"usps","tag":"Delivered","tracking_number":"9400111899223856922204"}}`)
  default:t.Errorf("unexpected request: %s %s",r.Method,r.URL.Path);w.WriteHeader(404)
  }
 }))
 defer ts.Close()
 api:=newAftershipAPI("test-secret");api.baseURL=ts.URL+"/trackings"
 id:=uuid.MustParse("00000000-0000-0000-0000-000000000001")
 first,err:=api.register(context.Background(),id,"9400111899223856922204","other")
 if err!=nil||first.Slug!="usps"||first.deliveryDate()!="2026-11-02"||len(first.Checkpoints)!=1{t.Fatalf("registered result %#v err %v",first,err)}
 second,err:=api.fetch(context.Background(),first.ID)
 if err!=nil||second.Tag!="Delivered"||created!=1||queried!=1{t.Fatalf("fetch %#v err %v counts %d/%d",second,err,created,queried)}
 if _,err=api.fetch(context.Background(),"../../config");err==nil{t.Fatal("path traversal accepted")}
 ref:=first.Checkpoints[0].reference()
 if ref!=first.Checkpoints[0].reference()||!strings.HasPrefix(ref,"aftership:"){t.Fatalf("unstable checkpoint reference %q",ref)}
 if first.Checkpoints[0].occurredAt().After(time.Now()){t.Fatal("checkpoint timestamp is in future")}
}
func TestAftershipAPIErrorsAreSanitized(t *testing.T){
 ts:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  w.WriteHeader(http.StatusUnauthorized)
  fmt.Fprint(w,"secret-key should never appear to users")
 }))
 defer ts.Close()
 api:=newAftershipAPI("hidden-key");api.baseURL=ts.URL
 _,err:=api.fetch(context.Background(),"tracking1")
 if err==nil||strings.Contains(err.Error(),"secret-key")||strings.Contains(err.Error(),"hidden-key"){t.Fatalf("provider secret leaked: %v",err)}
}
