package api

import (
 "context"
 "encoding/json"
 "fmt"
 "net/http"
 "net/http/httptest"
 "os"
 "testing"
 "time"

 "github.com/gigabytegrove/calden/internal/store"
 "github.com/google/uuid"
)

// The history API is read-only, permission-scoped and paginated. A shared
// viewer can inspect a shipment, but a private shipment must not leak.
func TestPackageTrackingHistoryPermissionsAndPagination(t *testing.T) {
 connection:=os.Getenv("CALDEN_TEST_DATABASE_URL")
 if connection=="" {t.Skip("requires isolated PostgreSQL test database")}
 ctx,cancel:=context.WithTimeout(context.Background(),90*time.Second)
 defer cancel()
 db,err:=store.Open(ctx,connection)
 if err!=nil {t.Fatal(err)}
 defer db.Close()
 if err:=store.Migrate(ctx,db);err!=nil {t.Fatal(err)}
 owner,viewer:=uuid.New(),uuid.New()
 for _,id:=range []uuid.UUID{owner,viewer} {
  _,err=db.Exec(ctx,"INSERT INTO users(id,username,display_name,password_hash,initials,role) VALUES($1,$2,'Tracking Test','test-only','TT','admin')",id,id.String())
  if err!=nil {t.Fatal(err)}
 }
 defer func(){
  _,_=db.Exec(context.Background(),"DELETE FROM users WHERE id=$1 OR id=$2",owner,viewer)
 }()
 _,err=db.Exec(ctx,"INSERT INTO app_settings(key,value) VALUES('packages_enabled','true') ON CONFLICT(key) DO UPDATE SET value='true'")
 if err!=nil {t.Fatal(err)}
 var id uuid.UUID
 err=db.QueryRow(ctx,
  "INSERT INTO package_shipments(owner_id,label,carrier,tracking_number,shared) VALUES($1,'History test parcel','ups',$2,false) RETURNING id",
  owner,uuid.New().String()).Scan(&id)
 if err!=nil {t.Fatal(err)}
 defer func(){_,_=db.Exec(context.Background(),"DELETE FROM package_shipments WHERE id=$1",id)}()
 timestamp:=time.Date(2026,10,28,12,0,0,0,time.UTC)
 for i:=0;i<105;i++{
  _,err=db.Exec(ctx,
    "INSERT INTO package_checkpoints(shipment_id,actor_id,status,source,detail,occurred_at) VALUES($1,$2,'in_transit','provider',$3,$4)",
    id,owner,fmt.Sprintf("Verified checkpoint %03d",i),timestamp.Add(time.Duration(i)*time.Second))
  if err!=nil {t.Fatal(err)}
 }
 s:=&server{db:db}
 request:=func(user uuid.UUID,offset string,child bool)*http.Request{
  req:=httptest.NewRequest(http.MethodGet,"/api/packages/"+id.String()+"/history"+offset,nil).
   WithContext(withActor(ctx,actor{ID:user,Role:"admin",IsChild:child}))
  req.SetPathValue("id",id.String())
  return req
 }
 receive:=func(user uuid.UUID,offset string,child bool)*httptest.ResponseRecorder{
  out:=httptest.NewRecorder()
  s.packageHistory(out,request(user,offset,child))
  return out
 }
 if got:=receive(viewer,"",false);got.Code!=http.StatusNotFound{
  t.Fatalf("private tracking history leaked to viewer: %d %s",got.Code,got.Body.String())
 }
 if got:=receive(owner,"",true);got.Code!=http.StatusForbidden{
  t.Fatalf("child could view package history: %d %s",got.Code,got.Body.String())
 }
 _,err=db.Exec(ctx,"UPDATE package_shipments SET shared=true WHERE id=$1",id)
 if err!=nil {t.Fatal(err)}
 type checkpoint struct {
  Source string `json:"source"`
  Detail string `json:"detail"`
 }
 type page struct {
  Items []checkpoint `json:"items"`
  HasMore bool `json:"has_more"`
  NextOffset int `json:"next_offset"`
 }
 readPage:=func(offset string)page{
  response:=receive(viewer,offset,false)
  if response.Code!=http.StatusOK{t.Fatalf("shared read: %d %s",response.Code,response.Body.String())}
  var result page
  if err:=json.Unmarshal(response.Body.Bytes(),&result);err!=nil {t.Fatal(err)}
  return result
 }
 first:=readPage("?offset=0")
 if len(first.Items)!=100||!first.HasMore||first.NextOffset!=100{
  t.Fatalf("first history page: count=%d hasMore=%v next=%d",len(first.Items),first.HasMore,first.NextOffset)
 }
 if first.Items[0].Detail!="Verified checkpoint 104"||first.Items[0].Source!="provider"{
  t.Fatalf("wrong first page ordering/source: %#v",first.Items[0])
 }
 second:=readPage("?offset=100")
 if len(second.Items)!=5||second.HasMore||second.NextOffset!=105{
  t.Fatalf("older history page: count=%d hasMore=%v next=%d",len(second.Items),second.HasMore,second.NextOffset)
 }
 if second.Items[4].Detail!="Verified checkpoint 000"{
  t.Fatalf("oldest verified activity missing: %#v",second.Items[4])
 }
 if got:=receive(viewer,"?offset=-1",false);got.Code!=http.StatusBadRequest{
  t.Fatalf("negative history offset accepted: %d",got.Code)
 }
}
