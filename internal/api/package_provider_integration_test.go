package api

import (
 "context"
 "net/http"
 "net/http/httptest"
 "os"
 "strings"
 "testing"
 "time"

 "github.com/google/uuid"
 "github.com/gigabytegrove/calden/internal/secrets"
 "github.com/gigabytegrove/calden/internal/store"
)
func TestPackageProviderDatabaseIntegration(t *testing.T){
 conn:=os.Getenv("CALDEN_TEST_DATABASE_URL")
 if conn==""{t.Skip("Requires isolated test PostgreSQL")}
 ctx,cancel:=context.WithTimeout(context.Background(),90*time.Second);defer cancel()
 db,err:=store.Open(ctx,conn);if err!=nil{t.Fatal(err)};defer db.Close()
 if err=store.Migrate(ctx,db);err!=nil{t.Fatal(err)}
 secret:=[]byte(strings.Repeat("P",48))
 s:=&server{db:db,jwtSecret:secret}
 owner:=uuid.New()
 _,err=db.Exec(ctx,"INSERT INTO users(id,username,display_name,password_hash,initials,role) VALUES($1,$2,'Tracking Tester','test-only','TT','admin')",owner,owner.String());if err!=nil{t.Fatal(err)}
 _,err=db.Exec(ctx,"INSERT INTO app_settings(key,value) VALUES('packages_enabled','true') ON CONFLICT(key) DO UPDATE SET value='true'");if err!=nil{t.Fatal(err)}
 req:=func(method,path,body string)*http.Request{return httptest.NewRequest(method,path,strings.NewReader(body)).WithContext(withActor(ctx,actor{ID:owner,Role:"admin"}))}
 denied:=httptest.NewRecorder()
 s.saveTrackingProvider(denied,httptest.NewRequest("PUT","/api/packages/provider",strings.NewReader(`{"api_key":"1234567890123456789","enabled":true}`)).WithContext(withActor(ctx,actor{ID:owner,Role:"member"})))
 if denied.Code!=403{t.Fatalf("non-admin accepted API configuration: %d",denied.Code)}
 write:=httptest.NewRecorder()
 s.saveTrackingProvider(write,req("PUT","/api/packages/provider",`{"api_key":"mock_aftership_api_key_very_secret","enabled":true}`))
 if write.Code!=200{t.Fatalf("could not save key: %d %s",write.Code,write.Body.String())}
 var sealed string
 if err=db.QueryRow(ctx,"SELECT sealed_api_key FROM package_tracking_provider WHERE id=1").Scan(&sealed);err!=nil{t.Fatal(err)}
 if strings.Contains(sealed,"mock_aftership_api_key_very_secret"){t.Fatal("plain provider key in database")}
 decoded,err:=secrets.Open(secret,sealed)
 if err!=nil||decoded!=trackingSecretPrefix+"mock_aftership_api_key_very_secret"{t.Fatalf("provider key decryption failed: %v",err)}
 info:=httptest.NewRecorder()
 s.trackingProviderInfo(info,req("GET","/api/packages/provider",""))
 if info.Code!=200||strings.Contains(info.Body.String(),"mock_aftership_api_key_very_secret"){t.Fatalf("provider settings leak: %s",info.Body.String())}
 var id uuid.UUID
 err=db.QueryRow(ctx,`INSERT INTO package_shipments(owner_id,label,carrier,tracking_number) VALUES($1,'Test parcel','ups','1Z999AA10123456784') RETURNING id`,owner).Scan(&id)
 if err!=nil{t.Fatal(err)}
 job,claimed,err:=claimTracking(ctx,db,id)
 if err!=nil||!claimed||job.Carrier!="ups"{t.Fatalf("first provider claim %#v %v %v",job,claimed,err)}
 _,again,err:=claimTracking(ctx,db,id)
 if err!=nil||again{t.Fatalf("three-hour throttling bypass: %v / %v",again,err)}
 var next time.Time
 if err=db.QueryRow(ctx,"SELECT next_tracking_check_at FROM package_shipments WHERE id=$1",id).Scan(&next);err!=nil{t.Fatal(err)}
 if next.Before(time.Now().Add(2*time.Hour+58*time.Minute)){t.Fatalf("next poll isn't 3 hours out: %v",next)}
 _,err=db.Exec(ctx,"UPDATE package_shipments SET status='delivered',next_tracking_check_at=now() WHERE id=$1",id)
 if err!=nil{t.Fatal(err)}
 _,again,err=claimTracking(ctx,db,id)
 if err!=nil||again{t.Fatalf("delivered parcel still polled: %v %v",again,err)}
 deleteResp:=httptest.NewRecorder()
 s.deleteTrackingProvider(deleteResp,req("DELETE","/api/packages/provider",""))
 if deleteResp.Code!=204{t.Fatalf("provider disconnect failed: %d",deleteResp.Code)}
 key,err:=s.trackingProviderKey(ctx)
 if err!=nil||key!=""{t.Fatalf("provider credentials persisted after disconnect: %q %v",key,err)}
}
