package api

import (
 "bytes"
 "context"
 "errors"
 "strings"
 "testing"
 "time"
)

func TestLocalTrackerStageAndCourierMapping(t *testing.T){
 for _,c:=range []struct{raw,want string}{
  {"in_transit","in_transit"},{"out_for_delivery","out_for_delivery"},
  {"delivered","delivered"},{"exception","exception"},
  {"pre_transit","label_created"},{"failed_attempt","exception"},
  {"unrecognized_word",""},
 }{
  if got:=normalizeLocalStage(c.raw);got!=c.want{t.Fatalf("stage %q mapped to %q not %q",c.raw,got,c.want)}
 }
 if normalizeLocalCarrier("fedex-express")!="fedex"||normalizeLocalCarrier("ups")!="ups"||normalizeLocalCarrier("unknown")!="other"{
  t.Fatal("carrier normalization failed")
 }
}
func TestLocalTrackerAcceptsRealCarrierScansOnly(t *testing.T){
 raw:=[]byte(`{"carrier":"ups","source":"ups","result":{"current_stage":"in_transit","expected_delivery":"2026-10-31","events":[{"time":"2026-10-27T12:30:00Z","stage":"in_transit","location":"Valdosta, GA","description":"Arrived at facility"},{"time":"2026-10-26T09:30:00Z","stage":"pre_transit","description":"Label created"}]}}`)
 result,err:=parseLocalScraperJSON(raw,"1Z999AA10123456784")
 if err!=nil||len(result.Result.Events)!=2||normalizeLocalStage(result.Result.CurrentStage)!="in_transit"{
  t.Fatalf("carrier events rejected %#v / %v",result,err)
 }
 if providerDate(result.Result.ExpectedDelivery)!="2026-10-31"{t.Fatal("valid ETA missing")}
 if result.Result.Events[0].Location!="Valdosta, GA"{t.Fatal("real scan location missing")}
 if localScanRef(result.Result.Events[0])!=localScanRef(result.Result.Events[0]){t.Fatal("unstable event dedupe identity")}
 if got:=localScanTime("2026-10-27T12:30:00Z");!got.Equal(time.Date(2026,10,27,12,30,0,0,time.UTC)){
  t.Fatalf("wrong scan timestamp: %s",got)
 }
}
func TestLocalTrackerDoesNotInventStatuses(t *testing.T){
 for _,raw:=range [][]byte{
  []byte("not json"),[]byte(`{"carrier":"ups","source":"ups","result":{}}`),
  []byte(`{"carrier":"ups","source":"ups","result":{"events":[]}}`),
  bytes.Repeat([]byte("a"),2<<20+1),
 }{
  if _,err:=parseLocalScraperJSON(raw,"1Z999AA10123456784");err==nil{t.Fatal("no tracking data should not be considered verified")}
 }
}
func TestLocalTrackerRejectsUntrustedNumberAndMissingEngine(t *testing.T){
 if _,err:=invokeLocalScraper(context.Background(),"../../etc/passwd","other");err==nil{t.Fatal("tracking number used as executable path")}
 old:=localScannerExecutable()
 if old==""{t.Fatal("missing default bundled engine path")}
}
func TestLocalTrackerBoundedOutput(t *testing.T){
 b:=&limitedTrackingBuffer{cap:12}
 if _,err:=b.Write([]byte("hello"));err!=nil{t.Fatal(err)}
 if _,err:=b.Write([]byte(strings.Repeat("x",20)));err==nil{t.Fatal("unbounded carrier response accepted")}
 if !errors.Is(errLocalScraperUnavailable,errLocalScraperUnavailable){t.Fatal("invalid sentinel")}
}
