package api

import "testing"

func TestPackageEmailDatesAreOnlyTentativeExplicitDeliveryHints(t *testing.T){
 tests:=[]struct{name,subject,body,expected string}{
  {"iso","Estimated delivery: 2026-10-30","Tracking 1Z999AA10123456784","2026-10-30"},
  {"month name","Arriving October 30, 2026","Amazon package TBA123456789012","2026-10-30"},
  {"weekday month","Expected delivery by Friday, Oct 30","USPS label 9400111899223856922204","2026-10-30"},
  {"no hint","October 30, 2026 sale","Tracking 1Z999AA10123456784",""},
  {"invalid","Estimated delivery: 2026-02-30","UPS 1Z999AA10123456784",""},
  {"too far","Scheduled delivery: 2029-10-27","Tracking 1Z999AA10123456784",""},
 }
 for _,tt:=range tests{
  t.Run(tt.name,func(t *testing.T){
   raw:="From: Shipping <notices@example.org>\r\nDate: Tue, 27 Oct 2026 12:00:00 -0400\r\nSubject: "+tt.subject+"\r\n\r\n"+tt.body
   got:=extractPackageEstimatedDate(raw)
   if got!=tt.expected{t.Fatalf("expected %q, got %q",tt.expected,got)}
  })
 }
}
