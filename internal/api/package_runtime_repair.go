package api

// Calden's self-repair uses pinned release artifacts in the existing writable
// data volume. It never needs root, apk, a Docker socket or an extra service.
import (
 "archive/tar"
 "compress/gzip"
 "context"
 "crypto/sha256"
 "encoding/hex"
 "errors"
 "fmt"
 "io"
 "net/http"
 "os"
 "os/exec"
 "path/filepath"
 "runtime"
 "strings"
 "sync"
 "time"
 "github.com/jackc/pgx/v5/pgxpool"
)
const parcelRuntimeTag="v1.5.0-alpha.13"
const parcelDownloadLimit int64=1200<<20
const parcelExpandedLimit int64=4096<<20

var parcelInstallMu sync.Mutex
var parcelInstallStatus=struct{sync.RWMutex;state,detail string}{state:"pending",detail:"Calden is preparing tracking automatically."}

func parcelStoreRoot()string{
 data:=strings.TrimSpace(os.Getenv("CALDEN_DATA_DIR"));if data==""{data="/data"}
 return filepath.Join(data,".calden-parcel")
}
func parcelActiveRoot()string{return filepath.Join(parcelStoreRoot(),"current","runtime")}
func parcelBundledCLI()string{return filepath.Join(parcelActiveRoot(),"parcel","node_modules","universal-parcel-scraper","dist","cli","index.js")}
func parcelBundledNode()string{return filepath.Join(parcelActiveRoot(),"node")}
func parcelBundledChromium()string{return filepath.Join(parcelActiveRoot(),"chromium","chromium")}
func parcelExists(path string)bool{info,err:=os.Stat(path);return err==nil&&info.Mode().IsRegular()}
func parcelInstallSnapshot()(string,string){parcelInstallStatus.RLock();defer parcelInstallStatus.RUnlock();return parcelInstallStatus.state,parcelInstallStatus.detail}
func parcelSetState(state,detail string){parcelInstallStatus.Lock();parcelInstallStatus.state=state;parcelInstallStatus.detail=detail;parcelInstallStatus.Unlock()}
func parcelRuntimeName()(string,error){
 if runtime.GOOS!="linux"||(runtime.GOARCH!="amd64"&&runtime.GOARCH!="arm64"){return "",errors.New("unsupported Linux tracking architecture")}
 return "calden-tracker-linux-"+runtime.GOARCH+".tar.gz",nil
}
func parcelReleaseURL(name string)string{return "https://github.com/gigabytegrove/calden/releases/download/"+parcelRuntimeTag+"/"+name}
func parcelClient()*http.Client{
 return &http.Client{Timeout:12*time.Minute,CheckRedirect:func(r *http.Request,previous []*http.Request)error{
  if len(previous)>8||r.URL.Scheme!="https"{return errors.New("unsafe tracker download redirect")}
  host:=strings.ToLower(r.URL.Hostname())
  if host!="github.com"&&host!="release-assets.githubusercontent.com"&&!strings.HasSuffix(host,".githubusercontent.com"){return errors.New("untrusted tracking release host")}
  return nil
 }}
}
func parcelExpectedChecksum(ctx context.Context,client *http.Client,name string)(string,error){
 req,err:=http.NewRequestWithContext(ctx,"GET",parcelReleaseURL("SHA256SUMS"),nil);if err!=nil{return "",err}
 res,err:=client.Do(req);if err!=nil{return "",err};defer res.Body.Close()
 if res.StatusCode!=200{return "",fmt.Errorf("checksum download HTTP %d",res.StatusCode)}
 raw,err:=io.ReadAll(io.LimitReader(res.Body,64<<10));if err!=nil{return "",err}
 for _,line:=range strings.Split(string(raw),"\n"){
  fields:=strings.Fields(line)
  if len(fields)<2||strings.TrimPrefix(fields[len(fields)-1],"*")!=name||len(fields[0])!=64{continue}
  if _,err:=hex.DecodeString(fields[0]);err==nil{return strings.ToLower(fields[0]),nil}
 }
 return "",errors.New("tracker is missing from signed release checksum inventory")
}
func parcelDownload(ctx context.Context,target string)error{
 name,err:=parcelRuntimeName();if err!=nil{return err}
 client:=parcelClient()
 expected,err:=parcelExpectedChecksum(ctx,client,name);if err!=nil{return err}
 req,err:=http.NewRequestWithContext(ctx,"GET",parcelReleaseURL(name),nil);if err!=nil{return err}
 res,err:=client.Do(req);if err!=nil{return err};defer res.Body.Close()
 if res.StatusCode!=200{return fmt.Errorf("tracker runtime download HTTP %d",res.StatusCode)}
 if res.ContentLength>parcelDownloadLimit{return errors.New("tracker runtime download exceeds safety limit")}
 file,err:=os.OpenFile(target,os.O_WRONLY|os.O_EXCL|os.O_CREATE,0600);if err!=nil{return err}
 hash:=sha256.New()
 n,copyErr:=io.Copy(io.MultiWriter(file,hash),io.LimitReader(res.Body,parcelDownloadLimit+1))
 closeErr:=file.Close()
 if copyErr!=nil{return copyErr};if closeErr!=nil{return closeErr}
 if n>parcelDownloadLimit{return errors.New("tracker runtime download exceeds safety limit")}
 if hex.EncodeToString(hash.Sum(nil))!=expected{return errors.New("tracker runtime failed checksum verification")}
 return nil
}
func parcelExtract(archive,destination string)error{
 f,err:=os.Open(archive);if err!=nil{return err};defer f.Close()
 gz,err:=gzip.NewReader(f);if err!=nil{return err};defer gz.Close()
 tr:=tar.NewReader(gz);var size int64
 for {
  entry,err:=tr.Next()
  if errors.Is(err,io.EOF){return nil};if err!=nil{return err}
  clean:=filepath.Clean(entry.Name)
  if strings.HasPrefix(entry.Name,"/")||strings.Contains(entry.Name,"\x00")||(clean!="runtime"&&!strings.HasPrefix(clean,"runtime/")){return errors.New("unsafe tracker archive path")}
  target:=filepath.Join(destination,clean)
  if clean=="runtime"&&entry.Typeflag!=tar.TypeDir{return errors.New("invalid runtime root")}
  switch entry.Typeflag {
  case tar.TypeDir:
   if err=os.MkdirAll(target,0700);err!=nil{return err}
  case tar.TypeReg,tar.TypeRegA:
   if entry.Size<0||entry.Size>parcelExpandedLimit-size{return errors.New("tracker archive exceeds unpacked limit")}
   size+=entry.Size
   if err=os.MkdirAll(filepath.Dir(target),0700);err!=nil{return err}
   out,err:=os.OpenFile(target,os.O_CREATE|os.O_WRONLY|os.O_EXCL,0600);if err!=nil{return err}
   _,copyErr:=io.CopyN(out,tr,entry.Size)
   closeErr:=out.Close()
   if copyErr!=nil{return copyErr};if closeErr!=nil{return closeErr}
   perm:=os.FileMode(0600);if entry.Mode&0111!=0{perm=0700}
   if err=os.Chmod(target,perm);err!=nil{return err}
  default:
   return errors.New("unsupported symbolic link or special file in tracker runtime")
  }
 }
}
func parcelRuntimeEnv(root string)[]string{
 lib:=filepath.Join(root,"runtime","lib")
 return []string{
  "PATH="+os.Getenv("PATH"),"HOME="+os.Getenv("HOME"),"LANG=C.UTF-8",
  "NODE_OPTIONS=--max-old-space-size=512",
  "LD_LIBRARY_PATH="+lib+":"+filepath.Join(lib,"pulseaudio")+":"+filepath.Join(root,"runtime","chromium"),
  "TRACKING_CHROMIUM_PATH="+filepath.Join(root,"runtime","chromium","chromium"),
  "FONTCONFIG_PATH="+filepath.Join(root,"runtime","etc","fonts"),
  "XDG_DATA_DIRS="+filepath.Join(root,"runtime","share"),
  "SCRAPER_PROVIDERS=ParcelsApp,Ship24,17TRACK,Postal Ninja,UPU",
 }
}
func parcelProbe(ctx context.Context,root string)bool{
 cli:=filepath.Join(root,"runtime","parcel","node_modules","universal-parcel-scraper","dist","cli","index.js")
 node:=filepath.Join(root,"runtime","node")
 browser:=filepath.Join(root,"runtime","chromium","chromium")
 if !parcelExists(cli)||!parcelExists(node)||!parcelExists(browser){return false}
 checkCtx,cancel:=context.WithTimeout(ctx,25*time.Second);defer cancel()
 cmd:=exec.CommandContext(checkCtx,node,cli,"detect","1Z999AA10123456784")
 cmd.Env=parcelRuntimeEnv(root)
 out,err:=cmd.Output()
 return err==nil&&strings.Contains(strings.ToLower(string(out)),"ups")
}
func parcelRepair(ctx context.Context)error{
 parcelInstallMu.Lock();defer parcelInstallMu.Unlock()
 if parcelExists("/opt/calden-parcel/node_modules/universal-parcel-scraper/dist/cli/index.js")&&parcelExists("/usr/bin/chromium"){
  parcelSetState("ready","Calden's integrated tracking runtime is ready.")
  return nil
 }
 root:=parcelStoreRoot();current:=filepath.Join(root,"current")
 if parcelProbe(ctx,current){parcelSetState("ready","Calden's tracking runtime is ready.");return nil}
 parcelSetState("installing","Calden is automatically repairing its tracking runtime.")
 if err:=os.MkdirAll(root,0700);err!=nil{return err}
 temp,err:=os.MkdirTemp(root,".prepare-");if err!=nil{return err};defer os.RemoveAll(temp)
 archive:=filepath.Join(temp,"asset.tar.gz")
 if err=parcelDownload(ctx,archive);err!=nil{return err}
 stage:=filepath.Join(temp,"unpacked")
 if err=os.MkdirAll(stage,0700);err!=nil{return err}
 if err=parcelExtract(archive,stage);err!=nil{return err}
 if !parcelProbe(ctx,stage){return errors.New("tracker runtime failed safe offline verification")}
 old:=filepath.Join(root,"previous")
 _=os.RemoveAll(old)
 if _,err=os.Stat(current);err==nil{if err=os.Rename(current,old);err!=nil{return err}}
 if err=os.Rename(stage,current);err!=nil{
  if _,e:=os.Stat(old);e==nil{_ = os.Rename(old,current)}
  return err
 }
 _=os.RemoveAll(old)
 parcelSetState("ready","Calden repaired its tracking runtime automatically.")
 return nil
}
func StartParcelRuntimeRepair(ctx context.Context,db *pgxpool.Pool){
 go func(){
  repair:=func(){
   if err:=parcelRepair(ctx);err!=nil{
    parcelSetState("retrying","Calden is retrying automatic tracking repair.")
   }else{
    _,_=db.Exec(ctx,"UPDATE package_shipments SET next_tracking_check_at=now() WHERE NOT archived AND status!='delivered' AND status_source!='provider'")
   }
  }
  repair()
  timer:=time.NewTicker(10*time.Minute);defer timer.Stop()
  for {
   select{
   case <-ctx.Done():return
   case <-timer.C:if state,_:=parcelInstallSnapshot();state!="ready"{repair()}
   }
  }
 }()
}
