package api

import (
 "archive/tar"
 "compress/gzip"
 "context"
 "os"
 "path/filepath"
 "strings"
 "testing"
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
