package api

import (
 "context"
 "os"
 "os/exec"
 "path/filepath"
 "strings"
 "testing"
 "time"

 "github.com/gigabytegrove/calden/internal/store"
 "github.com/google/uuid"
)

func TestLocalCarrierShipmentDatabaseIntegration(t *testing.T){
 dsn:=os.Getenv("CALDEN_TEST_DATABASE_URL")
 if dsn==""{t.Skip("No isolated PostgreSQL test database")}
 if _,err:=exec.LookPath("node");err!=nil{t.Skip("Node not installed in Go-only test environment")}
 ctx,cancel:=context.WithTimeout(context.Background(),90*time.Second);defer cancel()
 db,err:=store.Open(ctx,dsn)
 if err!=nil{t.Fatal(err)}
 defer db.Close()
 if err=store.Migrate(ctx,db);err!=nil{t.Fatal(err)}
 owner:=uuid.New()
 _,err=db.Exec(ctx,"INSERT INTO users(id,username,display_name,password_hash,initials,role) VALUES($1,$2,'Local Scanner Test','invalid-test','LS','admin')",owner,owner.String())
 if err!=nil{t.Fatal(err)}
 t.Cleanup(func(){
  cleanCtx,cancel:=context.WithTimeout(context.Background(),5*time.Second);defer cancel()
  _,_=db.Exec(cleanCtx,"DELETE FROM users WHERE id=$1",owner)
 })
 var shipment uuid.UUID
 err=db.QueryRow(ctx,`INSERT INTO package_shipments(owner_id,label,carrier,tracking_number)
 VALUES($1,'Local scan integration','ups','1Z999AA10123456784') RETURNING id`,owner).Scan(&shipment)
 if err!=nil{t.Fatal(err)}
 // A synthetic upstream CLI response exercises the production subprocess,
 // event extraction, SQL transaction and real status/history persistence.
 stub:=filepath.Join(t.TempDir(),"tracking-stub.cjs")
 content:=`if(process.argv[2]!=="track"||process.argv[3]!=="1Z999AA10123456784")process.exit(2);
if(process.env.CALDEN_DB_PASSWORD||process.env.CALDEN_JWT_SECRET||process.env.CALDEN_HUB_AFTERSHIP_KEY)process.exit(3);
console.log(JSON.stringify({carrier:"ups",source:"ups",result:{current_stage:"in_transit",expected_delivery:"2026-11-02",events:[
{stage:"in_transit",instant:"2026-10-27T12:25:00Z",time:"2026-10-27T08:25:00-04:00",location:"Valdosta, GA",description:"Arrived at facility"},
{stage:"registered",time:"2026-10-26T20:15:00Z",description:"Shipping label created"}
]}}));`
 if err=os.WriteFile(stub,[]byte(content),0600);err!=nil{t.Fatal(err)}
 t.Setenv("CALDEN_LOCAL_PARCEL_CLI",stub)
 t.Setenv("CALDEN_DB_PASSWORD","must-not-be-leaked")
 t.Setenv("CALDEN_JWT_SECRET","must-not-be-leaked")
 job:=dueTracking{ID:shipment,Carrier:"ups",Number:"1Z999AA10123456784"}
 s:=&server{db:db}
 if err=s.syncLocalScraperTracking(ctx,job);err!=nil{t.Fatal(err)}
 var status,source,eta,carrier,latest string
 err=db.QueryRow(ctx,`SELECT status,status_source,COALESCE(expected_date::text,''),carrier,
 COALESCE((SELECT detail FROM package_checkpoints WHERE shipment_id=package_shipments.id
 ORDER BY occurred_at DESC,id DESC LIMIT 1),'')
 FROM package_shipments WHERE id=$1`,shipment).Scan(&status,&source,&eta,&carrier,&latest)
 if err!=nil{t.Fatal(err)}
 if status!="in_transit"||source!="provider"||eta!="2026-11-02"||carrier!="ups"||!strings.Contains(latest,"Valdosta, GA"){
  t.Fatalf("Carrier tracking not persisted: status=%q source=%q date=%q carrier=%q latest=%q",status,source,eta,carrier,latest)
 }
 if err=s.syncLocalScraperTracking(ctx,job);err!=nil{t.Fatal(err)}
 var count int
 if err=db.QueryRow(ctx,"SELECT COUNT(*) FROM package_checkpoints WHERE shipment_id=$1",shipment).Scan(&count);err!=nil{t.Fatal(err)}
 if count!=2{t.Fatalf("Repeated carrier scans duplicated event history: count=%d",count)}
}
