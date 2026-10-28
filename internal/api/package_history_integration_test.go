package api

import (
 "context"
 "encoding/json"
 "fmt"
 "net/http"
 "net/http/httptest"
 "os"
 "strings"
 "testing"
 "time"

 "github.com/gigabytegrove/calden/internal/store"
 "github.com/google/uuid"
)

func TestPackageHistoryPaginationAndPermissions(t *testing.T) {
 dbURL:=os.Getenv("CALDEN_TEST_DATABASE_URL")
 if dbURL=="" {t.Skip("requires dedicated PostgreSQL test database")}
 ctx,cancel:=context.WithTimeout(context.Background(),90*time.Second)
 defer cancel()
 db,err:=store.Open(ctx,dbURL)
 if err!=nil {t.Fatal(err)}
 defer db.Close()
 if err=store.Migrate(ctx,db);err!=nil {t.Fatal(err)}
 owner,viewer:=uuid.New(),uuid.New()
 for _,user:=range []uuid.UUID{owner,viewer} {
  if _,err=db.Exec(ctx,`INSERT INTO users(id,username,display_name,password_hash,initials,role)
   VALUES($1,$2,'History Tester','test-only','HT','admin')`,user,user.String());err!=nil{t.Fatal(err)}
 }
 defer func(){
  _,_=db.Exec(context.Background(),"DELETE FROM users WHERE id=$1 OR id=$2",owner,viewer)
 }()
 if _,err=db.Exec(ctx,`INSERT INTO app_settings(key,value) VALUES('packages_enabled','true')
  ON CONFLICT(key) DO UPDATE SET value='true'`);err!=nil{t.Fatal(err)}
 var id uuid.UUID
 err=db.QueryRow(ctx,`INSERT INTO package_shipments(owner_id,label,carrier,tracking_number,status)
  VALUES($1,'History sample','ups',$2,'in_transit') RETURNING id`,
 owner,"1Z"+strings.ToUpper(strings.ReplaceAll(uuid.NewString(),"-",""))[:16]).Scan(&id)
 if err!=nil{t.Fatal(err)}
 for i:=0;i<103;i++{
  occurred:=time.Date(2026,10,28,0,0,0,0,time.UTC).Add(time.Duration(i)*time.Minute)
  if _,err=db.Exec(ctx,`INSERT INTO package_checkpoints(shipment_id,status,detail,source,occurred_at)
   VALUES($1,'in_transit',$2,'provider',$3)`,id,fmt.Sprintf("Verified carrier checkpoint %03d",i),occurred);err!=nil{t.Fatal(err)}
 }
 s:=&server{db:db}
 call:=func(user uuid.UUID,child bool,offset string)*httptest.ResponseRecorder{
  path:="/api/packages/"+id.String()+"/history"
  if offset!=""{path+="?offset="+offset}
  req:=httptest.NewRequest(http.MethodGet,path,nil).WithContext(withActor(ctx,actor{ID:user,Role:"admin",IsChild:child}))
  req.SetPathValue("id",id.String())
  w:=httptest.NewRecorder()
  s.packageHistory(w,req)
  return w
 }
 type event struct{
  Status string `json:"status"`
  Detail string `json:"detail"`
  Source string `json:"source"`
  OccurredAt time.Time `json:"occurred_at"`
 }
 type response struct {
  Items []event `json:"items"`
  HasMore bool `json:"has_more"`
  NextOffset int `json:"next_offset"`
 }
 first:=call(owner,false,"")
 if first.Code!=http.StatusOK {t.Fatalf("owner cannot view history: %d %s",first.Code,first.Body.String())}
 var page response
 if err=json.Unmarshal(first.Body.Bytes(),&page);err!=nil{t.Fatal(err)}
 if len(page.Items)!=100||!page.HasMore||page.NextOffset!=100{t.Fatalf("first page: %+v",page)}
 if page.Items[0].Detail!="Verified carrier checkpoint 102"||page.Items[99].Detail!="Verified carrier checkpoint 003"{
  t.Fatalf("carrier timeline has wrong order: first=%q last=%q",page.Items[0].Detail,page.Items[99].Detail)
 }
 if page.Items[0].Source!="provider"||page.Items[0].OccurredAt.IsZero(){t.Fatal("carrier provenance/time missing")}
 second:=call(owner,false,"100")
 if err=json.Unmarshal(second.Body.Bytes(),&page);err!=nil{t.Fatal(err)}
 if len(page.Items)!=3||page.HasMore||page.NextOffset!=103||page.Items[2].Detail!="Verified carrier checkpoint 000"{
  t.Fatalf("older history not available: %+v",page)
 }
 for _,bad:=range []string{"-1","hello","100001"}{
  w:=call(owner,false,bad)
  if w.Code!=http.StatusBadRequest{t.Fatalf("invalid offset %q returned %d",bad,w.Code)}
 }
 if w:=call(viewer,false,"");w.Code!=http.StatusNotFound{t.Fatalf("private history exposed to another member: %d",w.Code)}
 if _,err=db.Exec(ctx,"UPDATE package_shipments SET shared=true WHERE id=$1",id);err!=nil{t.Fatal(err)}
 if w:=call(viewer,false,"");w.Code!=http.StatusOK{t.Fatalf("authorized shared viewer cannot see history: %d",w.Code)}
 if w:=call(owner,true,"");w.Code!=http.StatusForbidden{t.Fatalf("child could view package history: %d",w.Code)}
}
