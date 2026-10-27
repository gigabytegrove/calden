package trackinghub

import (
 "bytes"
 "context"
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "fmt"
 "net/http"
 "net/http/httptest"
 "os"
 "strings"
 "testing"
 "time"

 "github.com/google/uuid"
 "github.com/jackc/pgx/v5/pgxpool"
)

func TestHubRequiresPrivateKey(t *testing.T){
 if _,err:=New(context.Background(),Config{});err==nil{t.Fatal("hub must not start without database")}
}
func TestHubMultiTenantPostgres(t *testing.T){
 dsn:=os.Getenv("CALDEN_TEST_DATABASE_URL")
 if dsn==""{t.Skip("Only runs with isolated PostgreSQL CI service")}
 ctx,cancel:=context.WithTimeout(context.Background(),60*time.Second);defer cancel()
 db,err:=pgxpool.New(ctx,dsn)
 if err!=nil{t.Fatal(err)}
 defer db.Close()
 var providerCalls int
 provider:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  if r.Header.Get("as-api-key")!="privileged-provider-key"{t.Error("provider token absent")}
  providerCalls++
  var payload struct{TrackingNumber string `json:"tracking_number"`}
  if r.Method=="POST"{
   if json.NewDecoder(r.Body).Decode(&payload)!=nil{t.Fatal("invalid provider body")}
  }else if r.Method=="GET" {payload.TrackingNumber="1Z999AA10123456784"}
  w.Header().Set("Content-Type","application/json")
  fmt.Fprintf(w,`{"data":{"id":"test-provider-id-%d","tracking_number":"%s","slug":"ups","tag":"InTransit","subtag_message":"Shipment moving"}}`,providerCalls,payload.TrackingNumber)
 }))
 defer provider.Close()
 h,err:=New(ctx,Config{
  DB:db,AfterShipKey:"privileged-provider-key",AllowEnrollment:true,
  MaxInstallationsPerDay:100,MaxShipmentsPerInstall:2,
  MaxCallsPerInstallPerDay:10,MaxGlobalCallsPerDay:20,
  Endpoint:provider.URL,HTTPClient:provider.Client(),
 })
 if err!=nil{t.Fatal(err)}
 server:=httptest.NewServer(h.Handler())
 defer server.Close()
 install1:=uuid.New();install2:=uuid.New()
 cred1:=strings.Repeat("a",64);cred2:=strings.Repeat("b",64)
 call:=func(token,path string,body any)(int,[]byte){
  data,_:=json.Marshal(body)
  req,_:=http.NewRequest("POST",server.URL+path,bytes.NewReader(data))
  req.Header.Set("Authorization","Bearer "+token);req.Header.Set("Content-Type","application/json")
  resp,e:=http.DefaultClient.Do(req);if e!=nil{t.Fatal(e)}
  defer resp.Body.Close()
  var result bytes.Buffer;_,_=result.ReadFrom(resp.Body)
  if strings.Contains(result.String(),"privileged-provider-key"){t.Fatal("central API key exposed to Calden installation")}
  return resp.StatusCode,result.Bytes()
 }
 if code,_:=call(cred1,"/v1/trackings",map[string]string{"id":uuid.NewString(),"tracking_number":"1Z999AA10123456784","carrier":"ups"});code!=401{t.Fatalf("unregistered client accepted: %d",code)}
 for _,pair:=range []struct{id uuid.UUID;token string}{{install1,cred1},{install2,cred2}}{
  code,out:=call(pair.token,"/v1/installations",map[string]string{"installation_id":pair.id.String()})
  if code!=201{t.Fatalf("enroll %s: HTTP %d %s",pair.id,code,out)}
 }
 id:=uuid.NewString()
 shipment:=map[string]string{"id":id,"tracking_number":"1Z999AA10123456784","carrier":"ups"}
 code,out:=call(cred1,"/v1/trackings",shipment)
 if code!=200{t.Fatalf("first carrier response: HTTP %d %s",code,out)}
 if providerCalls!=1{t.Fatalf("expected single provider call, got %d",providerCalls)}
 code,out=call(cred1,"/v1/trackings",shipment)
 if code!=200||!bytes.Contains(out,[]byte(`"cached":true`)){t.Fatalf("3-hour cache missing: %d %s",code,out)}
 if providerCalls!=1{t.Fatal("duplicate check drained central API quota")}
 code,out=call(cred2,"/v1/trackings",shipment)
 if code!=200||providerCalls!=2{t.Fatalf("shipment cache leaked across installations: %d %s",code,out)}
 if code,_=call(cred1,"/v1/trackings",map[string]string{"id":uuid.NewString(),"tracking_number":"1Z999AA10123456784","carrier":"ups"});code!=200{t.Fatalf("second shipment rejected: %d",code)}
 if code,_=call(cred1,"/v1/trackings",map[string]string{"id":uuid.NewString(),"tracking_number":"1Z999AA10123456784","carrier":"ups"});code!=429{t.Fatalf("shipment quota not enforced: %d",code)}
 if code,_=call(cred1,"/v1/installations",map[string]string{"installation_id":install2.String()});code!=409{t.Fatalf("credential swap accepted: %d",code)}
 keyhash:=sha256.Sum256([]byte(cred1))
 var digest string
 if err=db.QueryRow(ctx,"SELECT token_hash FROM hub_installations WHERE installation_id=$1",install1).Scan(&digest);err!=nil{t.Fatal(err)}
 if digest!=hex.EncodeToString(keyhash[:])||digest==cred1{t.Fatal("installation bearer credential stored unhashed")}
}
