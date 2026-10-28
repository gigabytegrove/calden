package api

import (
 "encoding/json"
 "errors"
 "os"
 "os/exec"
 "path/filepath"
 "reflect"
 "time"
)

// A successful runtime probe survives an in-app update (syscall.Exec) and
// container restart. This receipt is only a cache of a verified local probe,
// not a substitute for periodic rechecks or a source of executable content.
type parcelFileStamp struct {
 Path string `json:"path"`
 Size int64 `json:"size"`
 Modified int64 `json:"modified"`
 Mode uint32 `json:"mode"`
}
type parcelReadyReceipt struct {
 Schema int `json:"schema"`
 Kind string `json:"kind"`
 Files [3]parcelFileStamp `json:"files"`
}
func parcelReceiptPath() string {return filepath.Join(parcelStoreRoot(),"verified-runtime.json")}

func parcelRuntimePaths(kind string)([3]string,error){
 switch kind {
 case "managed":
  return [3]string{parcelBundledNode(),parcelBundledCLI(),parcelBundledChromium()},nil
 case "native":
  node,err:=exec.LookPath("node")
  if err!=nil{return [3]string{},err}
  return [3]string{node,"/opt/calden-parcel/node_modules/universal-parcel-scraper/dist/cli/index.js","/usr/bin/chromium"},nil
 default:
  return [3]string{},errors.New("invalid tracker runtime kind")
 }
}

func parcelFingerprint(kind string)(parcelReadyReceipt,error){
 paths,err:=parcelRuntimePaths(kind)
 if err!=nil{return parcelReadyReceipt{},err}
 receipt:=parcelReadyReceipt{Schema:1,Kind:kind}
 for i,path:=range paths {
  info,statErr:=os.Stat(path)
  if statErr!=nil{return parcelReadyReceipt{},statErr}
  if !info.Mode().IsRegular(){return parcelReadyReceipt{},errors.New("tracker runtime is not a regular file")}
  if i!=1&&info.Mode()&0111==0{return parcelReadyReceipt{},errors.New("tracker runtime executable bit missing")}
  receipt.Files[i]=parcelFileStamp{Path:path,Size:info.Size(),Modified:info.ModTime().UnixNano(),Mode:uint32(info.Mode().Perm())}
 }
 return receipt,nil
}

func parcelForgetReceipt(){
 _=os.Remove(parcelReceiptPath())
}
func parcelRememberReady(kind string) error {
 receipt,err:=parcelFingerprint(kind)
 if err!=nil{return err}
 bytes,err:=json.Marshal(receipt)
 if err!=nil{return err}
 root:=parcelStoreRoot()
 if err=os.MkdirAll(root,0700);err!=nil{return err}
 f,err:=os.CreateTemp(root,".verified-")
 if err!=nil{return err}
 defer os.Remove(f.Name())
 if err=f.Chmod(0600);err!=nil{_ =f.Close();return err}
 if _,err=f.Write(bytes);err!=nil{_ =f.Close();return err}
 if err=f.Sync();err!=nil{_ =f.Close();return err}
 if err=f.Close();err!=nil{return err}
 return os.Rename(f.Name(),parcelReceiptPath())
}
func parcelRestoreReady()bool{
 data,err:=os.ReadFile(parcelReceiptPath())
 if err!=nil||len(data)>4096{return false}
 var previous parcelReadyReceipt
 if json.Unmarshal(data,&previous)!=nil||previous.Schema!=1{return false}
 current,err:=parcelFingerprint(previous.Kind)
 if err!=nil||!reflect.DeepEqual(current,previous){return false}
 parcelChooseManaged(previous.Kind=="managed")
 parcelSetState("ready","Calden retained its verified tracking runtime across the update.")
 return true
}

const parcelReceiptRecheckInterval = 10*time.Minute
