package api

import (
 "context"
 "strings"
 "testing"
)

func TestPublicTrackingCarrierURLAndHostAllowlist(t *testing.T){
 for _,item:=range []struct{carrier,host string}{{"ups","www.ups.com"},{"fedex","www.fedex.com"},{"usps","tools.usps.com"}}{
  target,err:=publicTrackingURL(item.carrier,"1Z999AA10123456784")
  if err!=nil||!strings.HasPrefix(target,"https://"){t.Errorf("%s missing official link: %v",item.carrier,err)}
  if !trustedPublicTrackingHost(item.carrier,item.host){t.Errorf("%s trusted host rejected",item.carrier)}
  for _,evil:=range []string{"127.0.0.1","www.ups.com.evil.example","tracking-updates.evil.example"}{
   if trustedPublicTrackingHost(item.carrier,evil){t.Errorf("%s untrusted host accepted: %s",item.carrier,evil)}
  }
 }
 if _,err:=publicTrackingURL("amazon","TBA123456789012");err==nil{t.Fatal("Do not claim Amazon account-only TBA is a public API")}
}
func TestPublicTrackingParserNeedsVisibleMatchingEvidence(t *testing.T){
 number:="1Z999AA10123456784"
 cases:=[]struct{name,body,want string}{
  {"explicit","<body><h1>Tracking 1Z999AA10123456784</h1><p>Shipment status: Out for Delivery</p></body>","out_for_delivery"},
  {"heading","<body><p>Tracking 1Z999AA10123456784</p><h2>Delivered</h2></body>","delivered"},
  {"marketing","<body><h2>Tracking packages</h2><a>Delivered</a><p>1Z999AA10123456784</p></body>",""},
  {"wrong ID","<body><p>Tracking ABCDEFGHIJKLMNOPQR</p><h2>Delivered</h2></body>",""},
  {"JS template","<body><script>const fixture='1Z999AA10123456784 Shipment status: Delivered'</script><h1>Tracking</h1></body>",""},
  {"challenge","<body><h1>Tracking 1Z999AA10123456784</h1><p>Shipment status: Delivered</p><p>Verify you are human</p></body>",""},
  {"no status","<body><h1>Tracking 1Z999AA10123456784</h1><p>Open our app for updates.</p></body>",""},
 }
 for _,tt:=range cases{
  t.Run(tt.name,func(t *testing.T){
   got,ok:=parsePublicTrackingHTML("ups",number,tt.body)
   if tt.want==""{
    if ok{t.Fatalf("invented status %#v",got)}
   }else if !ok||got.Status!=tt.want{t.Fatalf("got %#v available=%v want=%q",got,ok,tt.want)}
  })
 }
}
func TestPublicTrackingKeylessDoesNotAttemptUnknownHosts(t *testing.T){
 _,err:=fetchPublicCarrierTracking(context.Background(),"other","secret.internal.invalid")
 if err==nil{t.Fatal("Unknown carrier should not trigger arbitrary network access")}
 if strings.Contains(err.Error(),"API key"){t.Fatal("Keyless fallback must not demand AfterShip")}
}
