package api

import (
 "context"
 "errors"
 "os"
 "path/filepath"
 "strings"
 "testing"

 "github.com/jackc/pgx/v5"
)

type parcelFixtureRow struct{enabled bool}
func(r parcelFixtureRow)Scan(dest ...any)error{
 if len(dest)!=1{return errors.New("expected one setting")}
 p,ok:=dest[0].(*bool)
 if !ok{return errors.New("expected boolean target")}
 *p=r.enabled
 return nil
}
type parcelFixtureSettings bool
func(q parcelFixtureSettings)QueryRow(_ context.Context,_ string,_ ...any)pgx.Row{
 return parcelFixtureRow{enabled:bool(q)}
}

func parcelWriteFixtureFile(t *testing.T,path,content string,mode os.FileMode){
 t.Helper()
 if err:=os.MkdirAll(filepath.Dir(path),0700);err!=nil{t.Fatal(err)}
 if err:=os.WriteFile(path,[]byte(content),mode);err!=nil{t.Fatal(err)}
}

func TestTrackerVerifiedReceiptSurvivesExecRestart(t *testing.T){
 oldMode:=parcelManagedSelected()
 oldState,oldDetail:=parcelInstallSnapshot()
 t.Cleanup(func(){parcelChooseManaged(oldMode);parcelSetState(oldState,oldDetail)})
 t.Setenv("CALDEN_DATA_DIR",t.TempDir())
 // Emulate portable runtimes that have already passed the offline checks.
 parcelWriteFixtureFile(t,parcelBundledNode(),"#!/bin/sh\necho UPS\n",0700)
 parcelWriteFixtureFile(t,parcelBundledCLI(),"// test fixture",0600)
 parcelWriteFixtureFile(t,parcelBundledChromium(),"#!/bin/sh\necho Chromium\n",0700)
 if err:=parcelRememberReady("managed");err!=nil{t.Fatal(err)}
 parcelChooseManaged(false)
 parcelSetState("pending","Calden is preparing tracking automatically.")
 parcelStartupVerify(context.Background(),parcelFixtureSettings(true))
 state,message:=parcelInstallSnapshot()
 if state!="ready"||!parcelManagedSelected()||!strings.Contains(message,"retained"){
  t.Fatalf("restart lost verified tracking readiness: state=%q message=%q managed=%v",state,message,parcelManagedSelected())
 }
 if !parcelRuntimePresent(){t.Fatal("restored runtime was not selectable")}
 if parcelTrackingPending(""){t.Fatal("saved verified runtime incorrectly blocks tracking")}
}

func TestTrackerReceiptInvalidatesOnRuntimeChange(t *testing.T){
 oldMode:=parcelManagedSelected()
 oldState,oldDetail:=parcelInstallSnapshot()
 t.Cleanup(func(){parcelChooseManaged(oldMode);parcelSetState(oldState,oldDetail)})
 t.Setenv("CALDEN_DATA_DIR",t.TempDir())
 parcelWriteFixtureFile(t,parcelBundledNode(),"#!/bin/sh\necho UPS\n",0700)
 parcelWriteFixtureFile(t,parcelBundledCLI(),"// test fixture",0600)
 parcelWriteFixtureFile(t,parcelBundledChromium(),"#!/bin/sh\necho Chromium\n",0700)
 if err:=parcelRememberReady("managed");err!=nil{t.Fatal(err)}
 parcelChooseManaged(false)
 if !parcelRestoreReady(){t.Fatal("valid receipt rejected")}
 parcelWriteFixtureFile(t,parcelBundledChromium(),"#!/bin/sh\nexit 1\n# corrupt browser\n",0700)
 parcelChooseManaged(false)
 if parcelRestoreReady(){t.Fatal("changed executable falsely trusted from previous update")}
 parcelForgetReceipt()
 if parcelRestoreReady(){t.Fatal("revoked receipt restored")}
}

func TestTrackerStartupHonorsDisabledFeatureEvenWithCachedRuntime(t *testing.T){
 oldMode:=parcelManagedSelected()
 oldState,oldDetail:=parcelInstallSnapshot()
 t.Cleanup(func(){parcelChooseManaged(oldMode);parcelSetState(oldState,oldDetail)})
 t.Setenv("CALDEN_DATA_DIR",t.TempDir())
 parcelWriteFixtureFile(t,parcelBundledNode(),"#!/bin/sh\necho UPS\n",0700)
 parcelWriteFixtureFile(t,parcelBundledCLI(),"// test fixture",0600)
 parcelWriteFixtureFile(t,parcelBundledChromium(),"#!/bin/sh\necho Chromium\n",0700)
 if err:=parcelRememberReady("managed");err!=nil{t.Fatal(err)}
 parcelSetState("pending","Calden is preparing tracking automatically.")
 parcelStartupVerify(context.Background(),parcelFixtureSettings(false))
 state,_:=parcelInstallSnapshot()
 if state!="disabled"{t.Fatalf("disabled package tracking unexpectedly bootstrapped: %q",state)}
}

func TestRepairErrorExplainsWhyInsteadOfGenericPreparing(t *testing.T){
 message:=parcelRepairMessage(errors.New("permission denied on writable data volume"))
 if !strings.Contains(message,"permission denied")||!strings.Contains(message,"retry"){
  t.Fatalf("repair cause hidden from user: %q",message)
 }
}
