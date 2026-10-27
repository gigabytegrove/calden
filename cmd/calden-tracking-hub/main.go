package main

import (
 "context"
 "errors"
 "fmt"
 "log"
 "net/http"
 "os"
 "os/signal"
 "strconv"
 "strings"
 "syscall"
 "time"

 "github.com/gigabytegrove/calden/internal/trackinghub"
 "github.com/jackc/pgx/v5/pgxpool"
)
func readSecret(name string)string{
 if file:=strings.TrimSpace(os.Getenv(name+"_FILE"));file!=""{
  bytes,err:=os.ReadFile(file)
  if err!=nil{log.Fatalf("Could not read %s_FILE: %v",name,err)}
  return strings.TrimSpace(string(bytes))
 }
 return strings.TrimSpace(os.Getenv(name))
}
func positiveEnv(name string,fallback int)int{
 value:=strings.TrimSpace(os.Getenv(name))
 if value==""{return fallback}
 n,err:=strconv.Atoi(value)
 if err!=nil||n<=0{log.Fatalf("%s must be a positive integer",name)}
 return n
}
func main(){
 key:=readSecret("CALDEN_HUB_AFTERSHIP_KEY")
 url:=readSecret("CALDEN_HUB_DATABASE_URL")
 if len(key)<12||url==""{log.Fatal("Calden Tracking Hub requires an authorized AfterShip API key and its own PostgreSQL database")}
 ctx,stop:=signal.NotifyContext(context.Background(),syscall.SIGINT,syscall.SIGTERM)
 defer stop()
 db,err:=pgxpool.New(ctx,url)
 if err!=nil{log.Fatalf("Hub database invalid: %v",err)}
 defer db.Close()
 testCtx,cancel:=context.WithTimeout(ctx,10*time.Second)
 if err=db.Ping(testCtx);err!=nil{log.Fatalf("Hub database unavailable: %v",err)}
 cancel()
 cfg:=trackinghub.Config{
  DB:db,AfterShipKey:key,
  AllowEnrollment:strings.EqualFold(strings.TrimSpace(os.Getenv("CALDEN_HUB_PUBLIC_ENROLLMENT")),"true"),
  MaxInstallationsPerDay:positiveEnv("CALDEN_HUB_MAX_INSTALLS_DAILY",100),
  MaxShipmentsPerInstall:positiveEnv("CALDEN_HUB_MAX_SHIPMENTS_PER_INSTALL",50),
  MaxCallsPerInstallPerDay:positiveEnv("CALDEN_HUB_MAX_API_CALLS_PER_INSTALL",400),
  MaxGlobalCallsPerDay:positiveEnv("CALDEN_HUB_MAX_API_CALLS_DAILY",5000),
 }
 hub,err:=trackinghub.New(ctx,cfg)
 if err!=nil{log.Fatalf("Hub initialization failed: %v",err)}
 port:=positiveEnv("CALDEN_HUB_PORT",8790)
 service:=&http.Server{
  Addr:fmt.Sprintf(":%d",port),
  Handler:hub.Handler(),ReadHeaderTimeout:5*time.Second,
  ReadTimeout:20*time.Second,WriteTimeout:32*time.Second,
  IdleTimeout:60*time.Second,MaxHeaderBytes:16384,
 }
 go func(){
  cleanup:=time.NewTicker(24*time.Hour);defer cleanup.Stop()
  for{select{case <-ctx.Done():return;case <-cleanup.C:
    pruneCtx,cancel:=context.WithTimeout(ctx,30*time.Second)
    if err:=hub.Prune(pruneCtx);err!=nil{log.Printf("Tracking Hub cleanup failed: %v",err)}
    cancel()
  }}
 }()
 go func(){
  <-ctx.Done()
  deadline,cancel:=context.WithTimeout(context.Background(),15*time.Second);defer cancel()
  _=service.Shutdown(deadline)
 }()
 log.Printf("Calden Tracking Hub ready on :%d (enrollment %s)",port,map[bool]string{true:"enabled",false:"disabled"}[cfg.AllowEnrollment])
 if err:=service.ListenAndServe();err!=nil&&!errors.Is(err,http.ErrServerClosed){log.Fatalf("Hub stopped: %v",err)}
}
