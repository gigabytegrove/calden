package api

import (
 "archive/tar"
 "compress/gzip"
 "context"
 "os"
 "path/filepath"
 "strings"
 "testing"
 "time"
 "github.com/gigabytegrove/calden/internal/store"
)
func writeTestTrackerTar(t *testing.T,entry string,kind byte) string {
 t.Helper()
 archive:=filepath.Join(t.TempDir(),"asset.tar.gz")
 output,err:=os.Create(archive);if err!=nil{t.Fatal(err)}
 gz:=gzip.NewWriter(output)
 tarWriter:=tar.NewWriter(gz)
 if err:=tarWriter.WriteHeader(&tar.Header{Name:entry,Mode:0755,Typeflag:kind});err!=nil{t.Fatal(err)}
 if err:=tarWriter.Close();err!=nil{t.Fatal(err)}
 if err:=gz.Close();err!=nil{t.Fatal(err)}
 if err:=output.Close();err!=nil{t.Fatal(err)}
 return archive
}
func TestTrackerRepairRejectsArchiveEscapeAndLinks(t *testing.T) {
 for _,tc:=range []struct{name string;kind byte}{
  {"../../etc/passwd",tar.TypeReg},
  {"runtime/../../etc/passwd",tar.TypeReg},
  {"/runtime/secret",tar.TypeReg},
  {"runtime/bin/node",tar.TypeSymlink},
 }{
  t.Run(tc.name,func(t *testing.T) {
   archive:=writeTestTrackerTar(t,tc.name,tc.kind)
   if err:=parcelExtract(archive,t.TempDir());err==nil{t.Fatal("unsafe archive entry was accepted")}
  })
 }
}
func TestTrackerRepairKeepsDataRootPrivate(t *testing.T) {
 data:=t.TempDir()
 t.Setenv("CALDEN_DATA_DIR",data)
 root:=parcelStoreRoot()
 if !strings.HasPrefix(root,data+string(os.PathSeparator)) {t.Fatal("runtime is outside data volume")}
 if strings.Contains(parcelReleaseURL("SHA256SUMS"),"http://"){t.Fatal("insecure tracker release URL")}
}
func TestTrackerRepairDoesNotAcceptMissingCLI(t *testing.T) {
 if parcelProbe(context.Background(),t.TempDir()){t.Fatal("missing CLI accepted as ready")}
}

func TestTrackerSelfRepairRespectsOptInSetting(t *testing.T){
 conn:=os.Getenv("CALDEN_TEST_DATABASE_URL")
 if conn==""{t.Skip("requires isolated PostgreSQL test database")}
 ctx,cancel:=context.WithTimeout(context.Background(),90*time.Second);defer cancel()
 db,err:=store.Open(ctx,conn);if err!=nil{t.Fatal(err)};defer db.Close()
 if err=store.Migrate(ctx,db);err!=nil{t.Fatal(err)}
 // Test both settings without changing other suites' global feature state.
 tx,err:=db.Begin(ctx);if err!=nil{t.Fatal(err)};defer tx.Rollback(ctx)
 for _,tc:=range []struct{value string;want bool}{{"false",false},{"true",true}}{
  _,err=tx.Exec(ctx,`INSERT INTO app_settings(key,value)
  VALUES('packages_enabled',$1) ON CONFLICT(key) DO UPDATE SET value=EXCLUDED.value`,tc.value)
  if err!=nil{t.Fatal(err)}
  got,err:=parcelTrackingEnabled(ctx,tx)
  if err!=nil{t.Fatal(err)}
  if got!=tc.want{t.Fatalf("packages_enabled=%s produced %v; expected %v",tc.value,got,tc.want)}
 }
}
