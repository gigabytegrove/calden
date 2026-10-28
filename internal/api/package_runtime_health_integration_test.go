package api

import (
 "context"
 "net/http"
 "net/http/httptest"
 "os"
 "strings"
 "testing"
 "time"

 "github.com/gigabytegrove/calden/internal/store"
 "github.com/google/uuid"
)

func TestPackagesHealthDoesNotReprobeAlreadyVerifiedTracker(t *testing.T){
 conn:=os.Getenv("CALDEN_TEST_DATABASE_URL")
 if conn==""{t.Skip("requires isolated PostgreSQL test database")}
 ctx,cancel:=context.WithTimeout(context.Background(),90*time.Second);defer cancel()
 db,err:=store.Open(ctx,conn);if err!=nil{t.Fatal(err)};defer db.Close()
 if err=store.Migrate(ctx,db);err!=nil{t.Fatal(err)}
 previousMode:=parcelManagedSelected()
 previousState,previousDetail:=parcelInstallSnapshot()
 t.Cleanup(func(){parcelChooseManaged(previousMode);parcelSetState(previousState,previousDetail)})
 t.Setenv("CALDEN_DATA_DIR",t.TempDir())
 // Simulate a worker-verified runtime whose CLI startup is currently slow
 // or temporarily fails. The HTTP health endpoint must not launch a second
 // inconsistent six-second probe on every page load.
 parcelWriteFixtureFile(t,parcelBundledNode(),"#!/bin/sh\nexit 1\n",0700)
 parcelWriteFixtureFile(t,parcelBundledCLI(),"// fixture",0600)
 parcelWriteFixtureFile(t,parcelBundledChromium(),"#!/bin/sh\nexit 1\n",0700)
 parcelChooseManaged(true)
 parcelSetState("ready","Tracker verified by recovery worker.")
 owner:=uuid.New()
 _,err=db.Exec(ctx,"INSERT INTO users(id,username,display_name,password_hash,initials,role) VALUES($1,$2,'Runtime Health','test-only','RH','admin')",owner,owner.String())
 if err!=nil{t.Fatal(err)}
 defer func(){_,_=db.Exec(context.Background(),"DELETE FROM users WHERE id=$1",owner)}()
 _,err=db.Exec(ctx,"INSERT INTO app_settings(key,value) VALUES('packages_enabled','true') ON CONFLICT(key) DO UPDATE SET value='true'")
 if err!=nil{t.Fatal(err)}
 req:=httptest.NewRequest(http.MethodGet,"/api/packages/tracker-health",nil).
  WithContext(withActor(ctx,actor{ID:owner,Role:"admin"}))
 health:=func()*httptest.ResponseRecorder{
  w:=httptest.NewRecorder()
  (&server{db:db}).packageTrackerHealth(w,req)
  return w
 }
 healthy:=health()
 if healthy.Code!=200||!strings.Contains(healthy.Body.String(),"\"ready\":true"){
  t.Fatalf("verified runtime was reported unavailable: %d %s",healthy.Code,healthy.Body.String())
 }
 if err:=os.Remove(parcelBundledNode());err!=nil{t.Fatal(err)}
 missing:=health()
 if missing.Code!=200||!strings.Contains(missing.Body.String(),"\"ready\":false")||
  !strings.Contains(missing.Body.String(),"\"state\":\"installing\""){
  t.Fatalf("missing executable was still considered healthy: %d %s",missing.Code,missing.Body.String())
 }
}
