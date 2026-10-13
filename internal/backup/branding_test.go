package backup

import (
    "archive/tar"
    "compress/gzip"
    "encoding/json"
    "os"
    "path/filepath"
    "testing"
    "time"
)

// Older backups use the original mixed-case product identifier.
// Changing the product name shown to families must never make those backups unusable.
func TestBackupAcceptsPreviousBrandCapitalization(t *testing.T) {
    for _, product := range []string{"CalDen", "Calden"} {
        t.Run(product,func(t *testing.T) {
            dir := t.TempDir()
            path := filepath.Join(dir, "backup.tar.gz")
            file, err := os.Create(path)
            if err != nil { t.Fatal(err) }
            gz := gzip.NewWriter(file)
            tarWriter := tar.NewWriter(gz)
            manifest,err:=json.Marshal(Manifest{
                FormatVersion:FormatVersion,
                Product:product,
                Version:"1.4.3",
                CreatedAt:time.Now().UTC(),
                DatabaseFile:"calden.dump",
            })
            if err!=nil {t.Fatal(err)}
            for _,entry:=range []struct{name string;data []byte}{
                {"manifest.json",manifest},
                {"calden.dump",[]byte("fake dump; only archive validation is tested")},
            } {
                if err:=tarWriter.WriteHeader(&tar.Header{
                    Name:entry.name,Mode:0600,Size:int64(len(entry.data)),Typeflag:tar.TypeReg,
                });err!=nil {t.Fatal(err)}
                if _,err:=tarWriter.Write(entry.data);err!=nil {t.Fatal(err)}
            }
            if err:=tarWriter.Close();err!=nil {t.Fatal(err)}
            if err:=gz.Close();err!=nil {t.Fatal(err)}
            if err:=file.Close();err!=nil {t.Fatal(err)}
            got,dump,err:=validateBundle(path,dir)
            if err!=nil {t.Fatalf("previously created %s backup rejected: %v",product,err)}
            if got.Product!=product {t.Errorf("product changed: got %q, want %q",got.Product,product)}
            if _,err:=os.Stat(dump);err!=nil {t.Fatal(err)}
            _=os.RemoveAll(filepath.Dir(dump))
        })
    }
}
