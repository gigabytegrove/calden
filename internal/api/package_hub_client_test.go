package api

import (
 "context"
 "encoding/json"
 "net/http"
 "net/http/httptest"
 "os"
 "strings"
 "testing"

 "github.com/google/uuid"
)
func TestHubCredentialStableAndSeparated(t *testing.T){
 secret:=[]byte(strings.Repeat("a",48))
 id,token,err:=hubCredential(secret)
 if err!=nil||len(token)!=64||id==uuid.Nil{t.Fatalf("invalid installation identity: %s %v",token,err)}
 id2,token2,_:=hubCredential(secret)
 if id!=id2||token!=token2{t.Fatal("same appliance secret must retain its hub identity after restart")}
 id3,token3,_:=hubCredential([]byte(strings.Repeat("b",48)))
 if id==id3||token==token3{t.Fatal("different homes cannot share identities")}
 if _,_,err=hubCredential([]byte("weak"));err==nil{t.Fatal("weak secret accepted")}
}
func TestHubClientEnrollThenFetch(t *testing.T){
 secret:=[]byte(strings.Repeat("c",48))
 expectedID,token,_:=hubCredential(secret)
 requests:=[]string{}
 srv:=httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  requests=append(requests,r.URL.Path)
  if r.Header.Get("Authorization")!="Bearer "+token{t.Error("invalid installation bearer token")}
  w.Header().Set("Content-Type","application/json")
  if r.URL.Path=="/v1/installations"{
   var body struct{InstallationID string `json:"installation_id"`}
   if json.NewDecoder(r.Body).Decode(&body)!=nil||body.InstallationID!=expectedID.String(){t.Error("invalid installation registration")}
   _,_=w.Write([]byte(`{"registered":true}`));return
  }
  if r.URL.Path=="/v1/trackings"{
   var body struct{Number string `json:"tracking_number"`}
   _=json.NewDecoder(r.Body).Decode(&body)
   if body.Number!="1Z999AA10123456784"{t.Error("wrong carrier tracking number")}
   _,_=w.Write([]byte(`{"tracking":{"id":"verified-id","tracking_number":"1Z999AA10123456784","slug":"ups","tag":"InTransit","checkpoints":[]},"cached":false}`))
   return
  }
  w.WriteHeader(404)
 }))
 defer srv.Close()
 job:=dueTracking{ID:uuid.New(),Carrier:"ups",Number:"1Z999AA10123456784"}
 out,err:=fetchHubTracking(context.Background(),srv.Client(),srv.URL,secret,job)
 if err!=nil||out.Tag!="InTransit"||out.Slug!="ups"{t.Fatalf("shared carrier response invalid %#v %v",out,err)}
 if len(requests)!=2||requests[0]!="/v1/installations"||requests[1]!="/v1/trackings"{t.Fatalf("requests: %#v",requests)}
}
func TestTrackingHubMustHaveHTTPSOrigin(t *testing.T){
 old,had:=os.LookupEnv("CALDEN_TRACKING_HUB_URL")
 t.Cleanup(func(){if had{os.Setenv("CALDEN_TRACKING_HUB_URL",old)}else{os.Unsetenv("CALDEN_TRACKING_HUB_URL")}})
 for _,value:=range []string{"http://127.0.0.1:8790","https://example.org/path","https://user:password@example.org","file:///etc/passwd","https://example.org?token=secret"}{
  os.Setenv("CALDEN_TRACKING_HUB_URL",value)
  if _,err:=trackingHubURL();err==nil{t.Fatalf("insecure endpoint accepted: %q",value)}
 }
 os.Setenv("CALDEN_TRACKING_HUB_URL","https://tracking.example.org/")
 got,err:=trackingHubURL();if err!=nil||got!="https://tracking.example.org"{t.Fatalf("valid hub URL rejected: %q %v",got,err)}
}
