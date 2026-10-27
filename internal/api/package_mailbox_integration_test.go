package api

import (
 "net/http"
 "context"
 "encoding/json"
 "net/http/httptest"
 "os"
 "strings"
 "testing"
 "time"

 "github.com/gigabytegrove/calden/internal/secrets"
 "github.com/gigabytegrove/calden/internal/store"
 "github.com/google/uuid"
)
func TestPackageDatabaseIntegration(t *testing.T){
 url:=os.Getenv("CALDEN_TEST_DATABASE_URL")
 if url==""{t.Skip("set CALDEN_TEST_DATABASE_URL to an isolated empty CI database")}
 ctx,cancel:=context.WithTimeout(context.Background(),120*time.Second)
 defer cancel()
 db,err:=store.Open(ctx,url)
 if err!=nil{t.Fatal(err)}
 defer db.Close()
 if err=store.Migrate(ctx,db);err!=nil{t.Fatalf("package migration failed: %v",err)}
 id:=uuid.New()
 _,err=db.Exec(ctx,"INSERT INTO users(id,username,display_name,password_hash,initials,role) VALUES($1,$2,'CI Package Tester','test-only','CP','admin')",id,id.String())
 if err!=nil{t.Fatal(err)}
 _,err=db.Exec(ctx,"INSERT INTO app_settings(key,value) VALUES('packages_enabled','true') ON CONFLICT(key) DO UPDATE SET value='true'")
 if err!=nil{t.Fatal(err)}
 secret:=[]byte("000000000000000000000000000000000000000000000000")
 s:=&server{db:db,jwtSecret:secret}
 raw:="From: UPS <notify@example.org>\r\nDate: Tue, 27 Oct 2026 12:00:00 -0400\r\nSubject: Estimated delivery: 2026-10-30\r\n\r\nTracking 1Z999AA10123456784"
 result,err:=s.storePackageMailRaw(ctx,id,raw)
 if err!=nil||result.Created!=1{t.Fatalf("first import: %#v / %v",result,err)}
 result,err=s.storePackageMailRaw(ctx,id,raw)
 if err!=nil||result.Created!=0||result.Skipped!=1{t.Fatalf("duplicate import: %#v / %v",result,err)}
 var date,status,source string
 err=db.QueryRow(ctx,"SELECT COALESCE(expected_date::text,''),status,status_source FROM package_shipments WHERE owner_id=$1",id).Scan(&date,&status,&source)
 if err!=nil||date!="2026-10-30"||status!="unknown"||source!="email"{t.Fatalf("unexpected shipment: %q / %q / %q / %v",date,status,source,err)}
 req:=func(method,path,body string)*http.Request{
  return httptest.NewRequest(method,path,strings.NewReader(body)).WithContext(withActor(ctx,actor{ID:id,Role:"admin"}))
 }
 setting:=`{"hostname":"imap.gmail.com","username":"test@gmail.com","password":"never-store-plaintext","enabled":true}`
 post:=httptest.NewRecorder()
 s.savePackageMailbox(post,req("PUT","/api/packages/mailbox",setting))
 if post.Code!=200{t.Fatalf("mailbox save failed: %d %s",post.Code,post.Body.String())}
 var encrypted string
 if err=db.QueryRow(ctx,"SELECT encrypted_password FROM package_mailboxes WHERE owner_id=$1",id).Scan(&encrypted);err!=nil{t.Fatal(err)}
 if strings.Contains(encrypted,"never-store-plaintext"){t.Fatal("mailbox secret stored in plaintext")}
 decoded,err:=secrets.Open(secret,encrypted)
 if err!=nil||decoded!=packageIMAPSecretPrefix+"never-store-plaintext"{t.Fatal("mailbox encryption roundtrip failed")}
 response:=httptest.NewRecorder()
 s.packageMailboxInfo(response,req("GET","/api/packages/mailbox",""))
 if response.Code!=200||strings.Contains(response.Body.String(),"never-store-plaintext"){t.Fatalf("mailbox GET leaked secret: %s",response.Body.String())}
 var visible map[string]any
 if err=json.Unmarshal(response.Body.Bytes(),&visible);err!=nil||visible["configured"]!=true{t.Fatal("mailbox summary missing")}
 deletion:=httptest.NewRecorder()
 s.deletePackageMailbox(deletion,req("DELETE","/api/packages/mailbox",""))
 if deletion.Code!=204{t.Fatalf("disconnect failed: %d",deletion.Code)}
 var count int
 if err=db.QueryRow(ctx,"SELECT count(*) FROM package_shipments WHERE owner_id=$1",id).Scan(&count);err!=nil||count!=1{t.Fatal("disconnect deleted package")}
 _,err=db.Exec(ctx,"UPDATE app_settings SET value='false' WHERE key='packages_enabled'")
 if err!=nil{t.Fatal(err)}
 result,err=s.storePackageMailRaw(ctx,id,"From: FedEx <ship@example.org>\r\nSubject: Another shipment\r\n\r\nFedEx 123456789012")
 if err!=nil||result.Created!=0{t.Fatalf("disabled Packages accepted email: %#v %v",result,err)}
}
