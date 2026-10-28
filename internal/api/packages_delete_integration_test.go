package api

import (
 "context"
 "encoding/json"
 "net/http"
 "net/http/httptest"
 "os"
 "strings"
 "testing"
 "time"

 "github.com/gigabytegrove/calden/internal/store"
 "github.com/google/uuid"
)

func TestPackagePermanentDeleteAndSimpleReadd(t *testing.T) {
 databaseURL:=os.Getenv("CALDEN_TEST_DATABASE_URL")
 if databaseURL=="" {t.Skip("set CALDEN_TEST_DATABASE_URL to a dedicated test database")}
 ctx,cancel:=context.WithTimeout(context.Background(),90*time.Second)
 defer cancel()
 db,err:=store.Open(ctx,databaseURL)
 if err!=nil {t.Fatal(err)}
 defer db.Close()
 if err=store.Migrate(ctx,db);err!=nil {t.Fatal(err)}
 owner,viewer:=uuid.New(),uuid.New()
 for _,user:=range []uuid.UUID{owner,viewer} {
  _,err=db.Exec(ctx,"INSERT INTO users(id,username,display_name,password_hash,initials,role) VALUES($1,$2,'Package Test','test-only','PT','admin')",user,user.String())
  if err!=nil {t.Fatal(err)}
 }
 _,err=db.Exec(ctx,"INSERT INTO app_settings(key,value) VALUES('packages_enabled','true') ON CONFLICT(key) DO UPDATE SET value='true'")
 if err!=nil {t.Fatal(err)}
 s:=&server{db:db,jwtSecret:[]byte("test-only-secret-00000000000000000000000")}
 request:=func(method,path,body string,user uuid.UUID)*http.Request{
  return httptest.NewRequest(method,path,strings.NewReader(body)).WithContext(withActor(ctx,actor{ID:user,Role:"admin"}))
 }
 payload:="{\"label\":\"Test headphones\",\"tracking_number\":\"1Z999AA10123456784\"}"
 create:=func()uuid.UUID {
  w:=httptest.NewRecorder()
  s.createPackage(w,request("POST","/api/packages",payload,owner))
  if w.Code!=http.StatusCreated {t.Fatalf("minimal package creation: %d %s",w.Code,w.Body.String())}
  var out map[string]string
  if err=json.Unmarshal(w.Body.Bytes(),&out);err!=nil {t.Fatal(err)}
  id,err:=uuid.Parse(out["id"])
  if err!=nil {t.Fatal(err)}
  return id
 }
 id:=create()
 var carrier,status string
 if err=db.QueryRow(ctx,"SELECT carrier,status FROM package_shipments WHERE id=$1",id).Scan(&carrier,&status);err!=nil {t.Fatal(err)}
 if carrier!="ups"||status!="unknown" {t.Fatalf("automatic defaults: %q %q",carrier,status)}
 w:=httptest.NewRecorder()
 req:=request("DELETE","/api/packages/"+id.String(),"",viewer)
 req.SetPathValue("id",id.String())
 s.deletePackage(w,req)
 if w.Code!=http.StatusNotFound {t.Fatalf("other owner deleted shipment: %d",w.Code)}
 w=httptest.NewRecorder()
 req=request("DELETE","/api/packages/"+id.String(),"",owner)
 req.SetPathValue("id",id.String())
 s.deletePackage(w,req)
 if w.Code!=http.StatusNoContent {t.Fatalf("delete: %d %s",w.Code,w.Body.String())}
 var count int
 if err=db.QueryRow(ctx,"SELECT count(*) FROM package_checkpoints WHERE shipment_id=$1",id).Scan(&count);err!=nil||count!=0 {t.Fatal("checkpoints were not removed",count,err)}
 second:=create()
 if second==id {t.Fatal("old package record was reused")}
 if _,err=db.Exec(ctx,"UPDATE package_shipments SET archived=true WHERE id=$1",second);err!=nil {t.Fatal(err)}
 third:=create()
 if third==second {t.Fatal("archived package record was reused")}
 if err=db.QueryRow(ctx,"SELECT count(*) FROM package_shipments WHERE owner_id=$1 AND tracking_number='1Z999AA10123456784'",owner).Scan(&count);err!=nil||count!=1 {
  t.Fatalf("archived duplicate still blocks re-add: count=%d error=%v",count,err)
 }
}
