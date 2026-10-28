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
 "log"
 "net/http"
 "os"
 "os/exec"
 "path/filepath"
 "runtime"
 "strings"
 "sync"
 "time"
 "github.com/jackc/pgx/v5"
 "github.com/jackc/pgx/v5/pgxpool"
)
const parcelRuntimeTag="v1.5.0-alpha.17"
const parcelDownloadLimit int64=1200<<20
const parcelExpandedLimit int64=4096<<20

var parcelInstallMu sync.Mutex
// A legacy engine may exist but be broken. A repaired managed installation
// must win over an unusable legacy CLI.
var parcelRuntimeSelection struct { sync.RWMutex; managed bool }
func parcelChooseManaged(managed bool) {
 parcelRuntimeSelection.Lock(); parcelRuntimeSelection.managed=managed; parcelRuntimeSelection.Unlock()
}
func parcelManagedSelected() bool {
 parcelRuntimeSelection.RLock(); defer parcelRuntimeSelection.RUnlock()
 return parcelRuntimeSelection.managed
}
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
// Probe actual executable behavior offline; never send a real tracking number.
func parcelProbeExecutables(ctx context.Context,node,cli,browser string,env []string)bool{
 if !parcelExists(node)||!parcelExists(cli)||!parcelExists(browser){return false}
 probeCtx,cancel:=context.WithTimeout(ctx,25*time.Second);defer cancel()
 cmd:=exec.CommandContext(probeCtx,node,cli,"detect","1Z999AA10123456784")
 cmd.Env=env
 out,err:=cmd.Output()
 if err!=nil||!strings.Contains(strings.ToLower(string(out)),"ups"){return false}
 browserCmd:=exec.CommandContext(probeCtx,browser,"--version")
 browserCmd.Env=env
 return browserCmd.Run()==nil
}
func parcelNativeProbe(ctx context.Context)bool{
 node,err:=exec.LookPath("node");if err!=nil{return false}
 return parcelProbeExecutables(ctx,node,
  "/opt/calden-parcel/node_modules/universal-parcel-scraper/dist/cli/index.js",
  "/usr/bin/chromium",
  []string{"PATH="+os.Getenv("PATH"),"HOME="+os.Getenv("HOME"),"LANG=C.UTF-8","NODE_OPTIONS=--max-old-space-size=512"})
}
func parcelProbe(ctx context.Context,root string)bool{
 cli:=filepath.Join(root,"runtime","parcel","node_modules","universal-parcel-scraper","dist","cli","index.js")
 node:=filepath.Join(root,"runtime","node")
 browser:=filepath.Join(root,"runtime","chromium","chromium")
 return parcelProbeExecutables(ctx,node,cli,browser,parcelRuntimeEnv(root))
}
func parcelRepair(ctx context.Context)error{
 parcelInstallMu.Lock();defer parcelInstallMu.Unlock()
 if parcelNativeProbe(ctx){
  parcelChooseManaged(false)
  parcelSetState("ready","Calden's integrated tracking runtime passed its offline checks.")
  if err:=parcelRememberReady("native");err!=nil{log.Printf("tracker: unable to persist verified native runtime: %v",err)}
  return nil
 }
 root:=parcelStoreRoot();current:=filepath.Join(root,"current")
 if parcelProbe(ctx,current){
  parcelChooseManaged(true)
  parcelSetState("ready","Calden's managed tracking runtime passed its offline checks.")
  if err:=parcelRememberReady("managed");err!=nil{log.Printf("tracker: unable to persist verified managed runtime: %v",err)}
  return nil
 }
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
 parcelChooseManaged(true)
 parcelSetState("ready","Calden repaired and verified its tracking runtime automatically.")
 if err:=parcelRememberReady("managed");err!=nil{log.Printf("tracker: unable to persist recovered runtime: %v",err)}
 return nil
}
// The package feature is opt-in. Do not download a large browser runtime
// while Package Tracking is disabled, even on a legacy binary-only install.
type parcelSettingsQuerier interface {
 QueryRow(context.Context,string,...any)pgx.Row
}
func parcelTrackingEnabled(ctx context.Context,query parcelSettingsQuerier)(bool,error){
 var enabled bool
 err:=query.QueryRow(ctx,`SELECT COALESCE(
  (SELECT value='true' FROM app_settings WHERE key='packages_enabled'),false)`).Scan(&enabled)
 return enabled,err
}
func parcelRuntimePresent()bool{
 if parcelManagedSelected(){
  return parcelExists(parcelBundledCLI())&&parcelExists(parcelBundledNode())&&parcelExists(parcelBundledChromium())
 }
 node,err:=exec.LookPath("node")
 return err==nil&&parcelExists(node)&&parcelExists("/opt/calden-parcel/node_modules/universal-parcel-scraper/dist/cli/index.js")&&parcelExists("/usr/bin/chromium")
}
// A missing keyless runtime is a pending installation, not a carrier failure.
// Explicit test/operator CLI overrides and an optional configured API provider
// bypass the managed runtime without ever requiring an extra service.
func parcelTrackingPending(providerKey string)bool{
 if providerKey!=""||strings.TrimSpace(os.Getenv("CALDEN_LOCAL_PARCEL_CLI"))!=""{return false}
 state,_:=parcelInstallSnapshot()
 return state!="ready"||!parcelRuntimePresent()
}
// A syscall.Exec application update resets Go memory while preserving /data.
// Recover known-good tracking readiness before the HTTP server starts so the
// Packages UI never reports a fresh install just because Calden was updated.
func parcelStartupVerify(ctx context.Context,query parcelSettingsQuerier){
 settingCtx,cancel:=context.WithTimeout(ctx,5*time.Second)
 enabled,err:=parcelTrackingEnabled(settingCtx,query)
 cancel()
 if err!=nil{
  parcelSetState("retrying","Calden cannot read the Package Tracking setting; it will retry.")
  log.Printf("tracker: startup settings read failed: %v",err)
  return
 }
 if !enabled {
  parcelSetState("disabled","Package Tracking is off.")
  return
 }
 if parcelRestoreReady(){return}
 // No receipt from an earlier version: test existing executables once,
 // locally, before accepting web requests. Downloads never block startup.
 probeCtx,stop:=context.WithTimeout(ctx,25*time.Second)
 defer stop()
 // Prefer an already repaired persistent runtime. Old Alpine containers
 // may still contain a broken legacy CLI, which must not delay recovery.
 if parcelExists(parcelBundledCLI())&&parcelProbe(probeCtx,filepath.Join(parcelStoreRoot(),"current")){
  parcelChooseManaged(true)
  parcelSetState("ready","Calden's existing tracking runtime is ready.")
  if err:=parcelRememberReady("managed");err!=nil{log.Printf("tracker: managed readiness receipt: %v",err)}
  return
 }
 if parcelExists("/opt/calden-parcel/node_modules/universal-parcel-scraper/dist/cli/index.js")&&parcelNativeProbe(probeCtx){
  parcelChooseManaged(false)
  parcelSetState("ready","Calden's existing tracking runtime is ready.")
  if err:=parcelRememberReady("native");err!=nil{log.Printf("tracker: native readiness receipt: %v",err)}
  return
 }
 parcelForgetReceipt()
 parcelSetState("installing","Calden is checking or repairing the tracker in the background.")
}

func parcelRepairMessage(err error)string{
 if err==nil{return ""}
 detail:=strings.TrimSpace(err.Error())
 if len(detail)>200{detail=detail[:200]}
 return "Automatic tracking repair failed: "+detail+". Calden will retry automatically."
}

func StartParcelRuntimeRepair(ctx context.Context,db *pgxpool.Pool){
 parcelStartupVerify(ctx,db)
 go func(){
  var nextRetry time.Time
  nextProbe:=time.Now().Add(parcelReceiptRecheckInterval)
  previouslyEnabled:=false
  repair:=func(){
   enabled,err:=parcelTrackingEnabled(ctx,db)
   if err!=nil{
    parcelSetState("retrying","Cannot read Package Tracking settings; Calden will try again.")
    log.Printf("tracker: settings poll failed: %v",err)
    return
   }
   if !enabled{
    previouslyEnabled=false
    nextRetry=time.Time{}
    nextProbe=time.Time{}
    parcelSetState("disabled","Package Tracking is off.")
    return
   }
   if !previouslyEnabled{
    nextRetry=time.Time{}
    if state,_:=parcelInstallSnapshot();state!="ready"{nextProbe=time.Time{}}
   }
   previouslyEnabled=true
   state,_:=parcelInstallSnapshot()
   if state=="ready"&&parcelRuntimePresent(){
    // Avoid repeatedly launching Node/Chromium every 30 seconds. A verified
    // receipt is checked against file metadata on every restart, and the
    // actual executables are re-probed every ten minutes while enabled.
    if time.Now().Before(nextProbe){return}
    healthy:=false
    kind:="native"
    if parcelManagedSelected(){
     kind="managed"
     healthy=parcelProbe(ctx,filepath.Join(parcelStoreRoot(),"current"))
    }else{
     healthy=parcelNativeProbe(ctx)
    }
    if healthy{
     nextProbe=time.Now().Add(parcelReceiptRecheckInterval)
     if err:=parcelRememberReady(kind);err!=nil{log.Printf("tracker: persist readiness: %v",err)}
     return
    }
    parcelForgetReceipt()
    parcelSetState("installing","Calden detected a damaged tracker and is repairing it automatically.")
   }
   if !nextRetry.IsZero()&&time.Now().Before(nextRetry){return}
   if err:=parcelRepair(ctx);err!=nil{
    nextRetry=time.Now().Add(10*time.Minute)
    parcelSetState("retrying",parcelRepairMessage(err))
    log.Printf("tracker: automatic repair failed (retry in 10m): %v",err)
    return
   }
   nextRetry=time.Time{}
   nextProbe=time.Now().Add(parcelReceiptRecheckInterval)
   // Only reactivate shipments after a genuinely unavailable tracker was
   // repaired. Never restart everyone's three-hour carrier schedule merely
   // because the application binary updated.
   _,err=db.Exec(ctx,`UPDATE package_shipments SET next_tracking_check_at=now()
      WHERE NOT archived AND status!='delivered'
      AND COALESCE((SELECT value='true' FROM app_settings WHERE key='packages_enabled'),false)`)
   if err!=nil{log.Printf("tracker: requeue after recovery failed: %v",err)}
  }
  repair()
  ticker:=time.NewTicker(30*time.Second)
  defer ticker.Stop()
  for{
   select{
   case <-ctx.Done():return
   case <-ticker.C:repair()
   }
  }
 }()
}
