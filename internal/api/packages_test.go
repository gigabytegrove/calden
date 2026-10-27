package api

import (
 "strings"
 "testing"
)

func TestPackageInputValidation(t *testing.T){
 base:=packageInput{Label:" Parcel ",Carrier:"UPS",TrackingNumber:"1z999aa10123456784",Status:"in_transit",ExpectedDate:"2026-11-02"}
 if !preparePackageInput(&base){t.Fatal("valid tracking package rejected")}
 if base.Carrier!="ups"||base.Label!="Parcel"||base.TrackingNumber!="1Z999AA10123456784"{t.Fatalf("input wasn't normalized: %#v",base)}
 for _,mutate:=range []struct{name string;change func(*packageInput)}{
  {"script-in-tracking",func(p *packageInput){p.TrackingNumber="<script>"}},
  {"bad-date",func(p *packageInput){p.ExpectedDate="2026-02-30"}},
  {"unknown-carrier",func(p *packageInput){p.Carrier="malicious"}},
  {"unknown-status",func(p *packageInput){p.Status="fake_scan"}},
  {"oversize-label",func(p *packageInput){p.Label=strings.Repeat("a",141)}},
 }{
  t.Run(mutate.name,func(t *testing.T){p:=base;mutate.change(&p);if preparePackageInput(&p){t.Fatal("invalid package accepted")}})
 }
}
func TestPackageEmailRecognizesCarriers(t *testing.T){
 for _,tc:=range []struct{name,content,carrier,id string}{
  {"ups","From: UPS <notify@example.org>\nSubject: Your box shipped\n\nYour UPS tracking number is 1Z999AA10123456784.","ups","1Z999AA10123456784"},
  {"usps","From: Notice <notice@example.org>\nSubject: USPS tracking\n\n9400111899223856922204","usps","9400111899223856922204"},
  {"fedex","From: FedEx <notice@example.org>\nSubject: Delivery\n\nYour FedEx number: 123456789012","fedex","123456789012"},
  {"amazon","From: Delivery <notice@example.org>\nSubject: Amazon package\n\nTBA123456789012","amazon","TBA123456789012"},
 }{
  t.Run(tc.name,func(t *testing.T){_,matches,err:=extractEmailPackages(tc.content);if err!=nil{t.Fatal(err)}
   for _,m:=range matches{if m.Carrier==tc.carrier&&m.TrackingNumber==tc.id{return}}
   t.Fatalf("expected %s:%s, got %#v",tc.carrier,tc.id,matches)
  })
 }
}
func TestPackageEmailUntrustedAndBounded(t *testing.T){
 _,matches,err:=extractEmailPackages("From: Scam <random@example.com>\nSubject: Test\n\n9999")
 if err!=nil||len(matches)!=0{t.Fatalf("unsupported tracking number must not be imported: %#v / %v",matches,err)}
 _,_,err=extractEmailPackages(strings.Repeat("A",262145))
 if err==nil{t.Fatal("oversized message accepted")}
 _,_,err=extractEmailPackages("not a MIME message")
 if err==nil{t.Fatal("invalid MIME message accepted")}
}
func TestPackageTrackingLinkHosts(t *testing.T){
 for _,carrier:=range []string{"ups","usps","fedex","amazon"}{
  link:=packageTrackingURL(carrier,"1Z3 TEST")
  if !strings.HasPrefix(link,"https://"){t.Fatalf("unsafe link %q",link)}
  if strings.Contains(link," TEST"){t.Fatalf("query string wasn't encoded: %q",link)}
 }
 if packageTrackingURL("other","abc")!=""{t.Fatal("unknown carrier needs no URL")}
}
