package api

import (
  "context"
  "encoding/json"
  "net/http"
  "net/http/httptest"
  "sync/atomic"
  "testing"
  "time"
)

func TestConflictOverlapUsesHalfOpenIntervals(t *testing.T) {
  base:=time.Date(2026,10,12,12,0,0,0,time.UTC)
  end:=base.Add(time.Hour)
  if scheduleOverlap(base,end,end,end.Add(time.Hour)){
    t.Fatal("adjacent appointments must not conflict")
  }
  if !scheduleOverlap(base,end,base.Add(30*time.Minute),end.Add(time.Hour)){
    t.Fatal("overlapping appointments not detected")
  }
  if scheduleOverlap(base,end,end.Add(time.Second),end.Add(time.Minute)){
    t.Fatal("non-overlapping dates must not conflict")
  }
}

func TestMonitaChannelDiscoveryValidatesScopedToken(t *testing.T) {
  var calls atomic.Int32
  server:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
    calls.Add(1)
    if r.URL.Path!="/service/channels" {t.Errorf("Unexpected path %q",r.URL.Path)}
    if r.Header.Get("Authorization")!="Bearer SA.integration-test-token" {
      t.Errorf("Missing scoped token")
    }
    w.Header().Set("Content-Type","application/json")
    _=json.NewEncoder(w).Encode([]monitaChannel{{ID:9,Name:"Family"}})
  }))
  defer server.Close()
  channels,err:=monitaAvailableChannels(context.Background(),
      monitaSettings{ServerURL:server.URL,Token:"SA.integration-test-token"})
  if err!=nil {t.Fatal(err)}
  if len(channels)!=1 || channels[0].ID!=9 {t.Fatal("Wrong channels")}
  if calls.Load()!=1{t.Fatal("Expected exactly one channel read")}
}

func TestMonitaPublisherUsesServiceChannel(t *testing.T){
  var calls atomic.Int32
  server:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
    calls.Add(1)
    if r.URL.Path!="/service/application/9/message" {
      t.Errorf("Publishing to wrong Monita channel: %s",r.URL.Path)
    }
    if r.Header.Get("Authorization")!="Bearer SA.integration-test-token" {
      t.Error("Missing scoped publish authorization")
    }
    if r.Method!="POST"{t.Error("Expected POST")}
    w.WriteHeader(201)
  }))
  defer server.Close()
  cfg:=monitaSettings{ServerURL:server.URL,Token:"SA.integration-test-token",ChannelID:9}
  if err:=sendMonita(cfg,"test","test notice");err!=nil{t.Fatal(err)}
  if calls.Load()!=1{t.Fatal("Duplicate Monita publish")}
}
