package api

import (
 "context"
 "errors"
 "html"
 "io"
 "net/http"
 "net/url"
 "regexp"
 "strings"
)

// Public tracking pages are intended for humans. The presence of a public
// URL does not imply that the carrier exposes machine-readable status.
// This adapter makes one ordinary unauthenticated HTTPS request, never
// solves challenges, submits credentials, or calls private app endpoints.
const publicTrackingUnavailable = "Live status is not available from this carrier's public page. Use Track with carrier for the official details."
const publicTrackingMaxBody = 768 * 1024

var publicTrackingTag = regexp.MustCompile("(?s)<[^>]{1,2048}>")
var publicTrackingScript = regexp.MustCompile("(?is)<(?:script|style|noscript)\\b[^>]*>.*?</(?:script|style|noscript)\\s*>")
var publicTrackingWhitespace = regexp.MustCompile(`\s+`)
var publicTrackingStatusPrefix = regexp.MustCompile(`(?i)\b(?:current status|shipment status|delivery status|tracking status|package status|your package is|your item was)\b\s*[:\-]?\s*(delivered|out for delivery|in transit|on its way|label created|shipping label created|pre-shipment|pre shipment|delivery attempted|exception|return to sender|available for pickup)\b`)
var publicTrackingStatusHeading = regexp.MustCompile(`(?i)<h[1-6][^>]{0,256}>\s*(?:<[^>]{1,256}>)*\s*(delivered|out for delivery|in transit(?: to next facility)?|shipping label created|pre-shipment|delivery attempted|delivery exception)\b`)
var publicTrackingNumberChars = regexp.MustCompile(`[^A-Z0-9]`)

type publicTrackingResult struct {
 Status string
 Detail string
 Source string
}
func publicTrackingURL(carrier,number string) (string,error) {
 encoded:=url.QueryEscape(number)
 switch carrier {
 case "ups": return "https://www.ups.com/track?tracknum="+encoded,nil
 case "usps": return "https://tools.usps.com/go/TrackConfirmAction?tLabels="+encoded,nil
 case "fedex": return "https://www.fedex.com/fedextrack/?trknbr="+encoded,nil
 case "amazon":return "",errors.New(publicTrackingUnavailable)
 default:return "",errors.New("Automatic lookup cannot identify this carrier. Choose a carrier manually or use Track with carrier.")
 }
}
func trustedPublicTrackingHost(carrier,host string)bool{
 host=strings.ToLower(host)
 switch carrier {
 case "ups":return host=="www.ups.com"||host=="ups.com"||host=="wwwapps.ups.com"
 case "usps":return host=="tools.usps.com"||host=="www.usps.com"
 case "fedex":return host=="www.fedex.com"||host=="fedex.com"||host=="wtrk.fedex.com"
 }
 return false
}
func normalizePublicStatus(raw string)string{
 switch strings.ToLower(strings.TrimSpace(raw)){
 case "delivered":return "delivered"
 case "out for delivery":return "out_for_delivery"
 case "in transit","in transit to next facility","on its way","available for pickup":return "in_transit"
 case "shipping label created","label created","pre-shipment","pre shipment":return "label_created"
 case "exception","delivery exception","delivery attempted","return to sender":return "exception"
 default:return ""
 }
}
// An untrusted HTML page can include generic strings such as "Delivered" in
// static templates, menus, translations or JavaScript. Never infer a status
// solely from a keyword. Require the exact tracking number in rendered text,
// a visible status label/heading close to it, and reject access challenges.
func parsePublicTrackingHTML(carrier,number,body string)(publicTrackingResult,bool){
 if len(body)==0||len(body)>publicTrackingMaxBody{return publicTrackingResult{},false}
 lowered:=strings.ToLower(body)
 for _,challenge:=range []string{"cf-challenge","captcha-delivery","ak-bot-check","access denied","verify you are human","checking your browser","pardon our interruption"}{
  if strings.Contains(lowered,challenge){return publicTrackingResult{},false}
 }
 withoutScripts:=publicTrackingScript.ReplaceAllString(body," ")
 text:=html.UnescapeString(publicTrackingTag.ReplaceAllString(withoutScripts," "))
 text=publicTrackingWhitespace.ReplaceAllString(text," ")
 upper:=strings.ToUpper(text)
 wanted:=normalizePackageTrackingNumber(number)
 if len(wanted)<4{return publicTrackingResult{},false}
 positions:=[]int{}
 // Some pages space the numeric tracking ID. Permit spaces/dashes but
 // never inspect script-embedded examples or unrelated tracker IDs.
 for _,index:=range allOccurrences(upper,wanted,8){positions=append(positions,index)}
 if len(positions)==0{return publicTrackingResult{},false}
 for _,index:=range positions{
  start:=index-300;if start<0{start=0}
  end:=index+1000;if end>len(text){end=len(text)}
  segment:=text[start:end]
  match:=publicTrackingStatusPrefix.FindStringSubmatch(segment)
  if len(match)!=2{continue}
  status:=normalizePublicStatus(match[1])
  if status==""{continue}
  return publicTrackingResult{Status:status,Detail:"Public tracking page: "+strings.TrimSpace(match[0]),Source:"public_page"},true
 }
 // Headings have to be close to the literal tracking number in the HTML;
 // this excludes unrelated marketing banners elsewhere on the page.
 for _,m:=range publicTrackingStatusHeading.FindAllStringSubmatchIndex(withoutScripts,12){
  if len(m)<4{continue}
  left:=m[0]-1500;if left<0{left=0}
  right:=m[1]+1500;if right>len(withoutScripts){right=len(withoutScripts)}
  near:=html.UnescapeString(publicTrackingTag.ReplaceAllString(withoutScripts[left:right]," "))
  normalized:=publicTrackingNumberChars.ReplaceAllString(strings.ToUpper(near),"")
  if !strings.Contains(normalized,wanted){continue}
  status:=normalizePublicStatus(withoutScripts[m[2]:m[3]])
  if status!=""{return publicTrackingResult{Status:status,Detail:"Status shown on official public tracking page",Source:"public_page"},true}
 }
 return publicTrackingResult{},false
}
func allOccurrences(text,needle string,limit int)[]int{
 positions:=[]int{}
 from:=0
 for len(positions)<limit{
  n:=strings.Index(text[from:],needle)
  if n<0{break}
  pos:=from+n
  if (pos==0||!isPublicTrackingChar(text[pos-1]))&&(pos+len(needle)==len(text)||!isPublicTrackingChar(text[pos+len(needle)])){
   positions=append(positions,pos)
  }
  from=pos+len(needle)
  if from>=len(text){break}
 }
 return positions
}
func isPublicTrackingChar(ch byte)bool{return ch>='A'&&ch<='Z'||ch>='0'&&ch<='9'}
func fetchPublicCarrierTracking(ctx context.Context,carrier,number string)(publicTrackingResult,error){
 target,err:=publicTrackingURL(carrier,number)
 if err!=nil{return publicTrackingResult{},err}
 u,err:=url.Parse(target)
 if err!=nil||u.Scheme!="https"||!trustedPublicTrackingHost(carrier,u.Hostname()){return publicTrackingResult{},errors.New(publicTrackingUnavailable)}
 client:=&http.Client{
  Timeout:12*time.Second,
  CheckRedirect:func(req *http.Request,prior []*http.Request)error{
   if len(prior)>=3||req.URL.Scheme!="https"||!trustedPublicTrackingHost(carrier,req.URL.Hostname()){
    return http.ErrUseLastResponse
   }
   return nil
  },
 }
 req,err:=http.NewRequestWithContext(ctx,http.MethodGet,target,nil)
 if err!=nil{return publicTrackingResult{},errors.New(publicTrackingUnavailable)}
 req.Header.Set("User-Agent","Calden/1.5 (+public-shipment-status; no authentication)")
 req.Header.Set("Accept","text/html,application/xhtml+xml")
 res,err:=client.Do(req)
 if err!=nil{return publicTrackingResult{},errors.New(publicTrackingUnavailable)}
 defer res.Body.Close()
 if res.StatusCode!=http.StatusOK{return publicTrackingResult{},errors.New(publicTrackingUnavailable)}
 if !strings.Contains(strings.ToLower(res.Header.Get("Content-Type")),"text/html"){return publicTrackingResult{},errors.New(publicTrackingUnavailable)}
 data,err:=io.ReadAll(io.LimitReader(res.Body,publicTrackingMaxBody+1))
 if err!=nil||len(data)>publicTrackingMaxBody{return publicTrackingResult{},errors.New(publicTrackingUnavailable)}
 result,ok:=parsePublicTrackingHTML(carrier,number,string(data))
 if !ok{return publicTrackingResult{},errors.New(publicTrackingUnavailable)}
 return result,nil
}
