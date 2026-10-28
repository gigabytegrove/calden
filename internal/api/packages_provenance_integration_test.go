package api

import (
 "context"
 "net/http"
 "net/http/httptest"
 "os"
 "strings"
 "testing"
 "time"

 "github.com/gigabytegrove/calden/internal/store"
 "github.com/google/uuid"
)

// Status-only snapshots are history, not a physical carrier scan. This tests
// the read API and the latest-scan SQL independently of UI copy.
func TestCarrierStatusSnapshotDoesNotCountAsPhysicalScan(t *testing.T){
 connection:=os.Getenv("CALDEN_TEST_DATABASE_URL")
 if connection==""{t.Skip("requires isolated PostgreSQL test database")}
 ctx,cancel:=context.WithTimeout(context.Background(),90*time.Second);defer cancel()
 db,err:=store.Open(ctx,connection);if err!=nil{t.Fatal(err)};defer db.Close()
 if err:=store.Migrate(ctx,db);err!=nil{t.Fatal(err)}
 owner:=uuid.New()
 _,err=db.Exec(ctx,"INSERT INTO users(id,username,display_name,password_hash,initials,role) VALUES($1,$2,'Carrier Provenance','test-only','CP','admin')",owner,owner.String())
 if err!=nil{t.Fatal(err)}
 defer func(){_,_=db.Exec(context.Background(),"DELETE FROM users WHERE id=$1",owner)}()
 _,err=db.Exec(ctx,"INSERT INTO app_settings(key,value) VALUES('packages_enabled','true') ON CONFLICT(key) DO UPDATE SET value='true'")
 if err!=nil{t.Fatal(err)}
 var id uuid.UUID
 err=db.QueryRow(ctx,"INSERT INTO package_shipments(owner_id,label,carrier,tracking_number,status,status_source) VALUES($1,'Source test','ups',$2,'in_transit','provider') RETURNING id",owner,"1Z"+strings.ToUpper(strings.ReplaceAll(uuid.NewString(),"-",""))[:16]).Scan(&id)
 if err!=nil{t.Fatal(err)}
 _,err=db.Exec(ctx,"INSERT INTO package_checkpoints(shipment_id,status,detail,source,provider_ref) VALUES($1,'in_transit','Carrier reports in transit','provider_status','local:current:in_transit')",id)
 if err!=nil{t.Fatal(err)}
 read:=func()packageRecord{
  record,err:=scanPackage(db.QueryRow(ctx,"SELECT "+packageColumns+" FROM package_shipments WHERE id=$1",id),owner)
  if err!=nil{t.Fatal(err)}
  return record
 }
 if got:=read().LatestScan;got!=""{t.Fatalf("status-only report fabricated a carrier scan: %s",got)}
 request:=httptest.NewRequest(http.MethodGet,"/api/packages/"+id.String()+"/history",nil).
  WithContext(withActor(ctx,actor{ID:owner,Role:"admin"}))
 request.SetPathValue("id",id.String())
 result:=httptest.NewRecorder()
 (&server{db:db}).packageHistory(result,request)
 if result.Code!=200||!strings.Contains(result.Body.String(),"\"source\":\"provider_status\""){
  t.Fatalf("status snapshot missing from history: %d %s",result.Code,result.Body.String())
 }
 _,err=db.Exec(ctx,"INSERT INTO package_checkpoints(shipment_id,status,detail,source,provider_ref) VALUES($1,'in_transit','Arrived at sorting facility','provider','local:actual:event')",id)
 if err!=nil{t.Fatal(err)}
 if got:=read().LatestScan;got!="Arrived at sorting facility"{t.Fatalf("actual carrier event not displayed: %s",got)}
}
