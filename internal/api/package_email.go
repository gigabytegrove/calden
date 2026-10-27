package api

import (
 "crypto/rand"
 "crypto/sha256"
 "encoding/base64"
 "encoding/hex"
 "errors"
 "html"
 "io"
 "mime"
 "mime/multipart"
 "mime/quotedprintable"
 "net/http"
 "net/mail"
 "regexp"
 "strings"

 "github.com/google/uuid"
 "github.com/jackc/pgx/v5"
)

var (
 packageUPS=regexp.MustCompile(`(?i)\b1Z[0-9A-Z]{16}\b`)
 packageUSPS=regexp.MustCompile(`(?i)\b(?:9[2345][0-9]{18,24}|[A-Z]{2}[0-9]{9}US)\b`)
 packageFedEx=regexp.MustCompile(`\b(?:[0-9]{12}|[0-9]{15}|[0-9]{20}|[0-9]{22})\b`)
 packageAmazon=regexp.MustCompile(`(?i)\bTBA[0-9]{10,20}\b`)
 packageHTMLTags=regexp.MustCompile(`(?s)<[^>]*>`)
 packageSpaces=regexp.MustCompile(`\s+`)
)
type incomingPackage struct {Carrier,TrackingNumber string}
func extractEmailPackages(raw string)(string,[]incomingPackage,error){
 if len(raw)>262144{return "",nil,errors.New("Shipping message is too large (256 KB maximum)")}
 msg,err:=mail.ReadMessage(strings.NewReader(raw))
 if err!=nil{return "",nil,errors.New("Paste or forward a complete RFC 822 email, including its headers")}
 header:=new(mime.WordDecoder)
 subject:=msg.Header.Get("Subject")
 if decoded,e:=header.DecodeHeader(subject);e==nil{subject=decoded}
 subject=packageSpaces.ReplaceAllString(strings.TrimSpace(subject)," ")
 if len(subject)>140{subject=subject[:140]}
 if subject==""{subject="Imported delivery notification"}
 body:=readPackageMailBody(msg.Header.Get("Content-Type"),msg.Header.Get("Content-Transfer-Encoding"),msg.Body)
 combined:=subject+"
"+body
 if len(combined)>262144{combined=combined[:262144]}
 // Email can be forged. The extracted data is only a suggestion: it is NEVER
 // marked as an authenticated carrier scan or used to overwrite a manual status.
 lower:=strings.ToLower(combined+"
"+msg.Header.Get("From"))
 seen:=map[string]bool{}
 matches:=make([]incomingPackage,0)
 add:=func(carrier string, re *regexp.Regexp){
  for _,number:=range re.FindAllString(combined,25){
   number=strings.ToUpper(strings.TrimSpace(number))
   k:=carrier+":"+number
   if !seen[k]{seen[k]=true;matches=append(matches,incomingPackage{carrier,number})}
  }
 }
 add("ups",packageUPS)
 add("amazon",packageAmazon)
 add("usps",packageUSPS)
 if strings.Contains(lower,"fedex")||strings.Contains(lower,"fed ex"){add("fedex",packageFedEx)}
 if len(matches)>25{matches=matches[:25]}
 return subject,matches,nil
}
func readPackageMailBody(contentType,transfer string,body io.Reader)string{
 // Reject binary attachments and read only bounded textual MIME parts.
 if strings.HasPrefix(strings.ToLower(contentType),"multipart/"){
  _,params,err:=mime.ParseMediaType(contentType)
  if err!=nil||params["boundary"]==""{return ""}
  mr:=multipart.NewReader(body,params["boundary"])
  var plain,markup string
  for i:=0;i<24;i++{
   part,e:=mr.NextPart();if e!=nil{break}
   kind:=strings.ToLower(part.Header.Get("Content-Type"))
   if strings.Contains(kind,"attachment")||part.FileName()!=""{part.Close();continue}
   b:=readPackageMailBody(kind,part.Header.Get("Content-Transfer-Encoding"),io.LimitReader(part,131072))
   part.Close()
   if strings.HasPrefix(kind,"text/plain"){plain+="
"+b} else if strings.HasPrefix(kind,"text/html"){markup+="
"+b}else if strings.HasPrefix(kind,"multipart/"){plain+="
"+b}
  }
  if strings.TrimSpace(plain)!=""{return plain}
  return markup
 }
 ct:=strings.ToLower(contentType)
 if ct!=""&&!strings.HasPrefix(ct,"text/plain")&&!strings.HasPrefix(ct,"text/html"){return ""}
 var reader io.Reader=io.LimitReader(body,131072)
 switch strings.ToLower(strings.TrimSpace(transfer)){
 case "base64":reader=base64.NewDecoder(base64.StdEncoding,reader)
 case "quoted-printable":reader=quotedprintable.NewReader(reader)
 }
 b,_:=io.ReadAll(io.LimitReader(reader,131072))
 out:=string(b)
 if strings.HasPrefix(ct,"text/html"){out=html.UnescapeString(packageHTMLTags.ReplaceAllString(out," "))}
 return out
}
func (s *server) ingestPackageEmail(w http.ResponseWriter,r *http.Request,owner uuid.UUID,raw string){
 subject,found,err:=extractEmailPackages(raw)
 if err!=nil{writeError(w,400,err.Error());return}
 if len(found)==0{writeError(w,422,"No recognizable Amazon, USPS, UPS or FedEx tracking numbers were found. Add the package manually.");return}
 tx,err:=s.db.Begin(r.Context())
 if err!=nil{writeError(w,500,"Could not import shipping email");return}
 defer tx.Rollback(r.Context())
 created,skipped:=0,0
 for _,p:=range found{
  if !packageTrackingPattern.MatchString(p.TrackingNumber){continue}
  var id uuid.UUID
  e:=tx.QueryRow(r.Context(),"INSERT INTO package_shipments(owner_id,label,carrier,tracking_number,status,status_source) VALUES($1,$2,$3,$4,'unknown','email') ON CONFLICT DO NOTHING RETURNING id",owner,subject,p.Carrier,p.TrackingNumber).Scan(&id)
  if errors.Is(e,pgx.ErrNoRows){skipped++;continue}
  if e!=nil{writeError(w,500,"Could not import shipping email");return}
  _,e=tx.Exec(r.Context(),"INSERT INTO package_checkpoints(shipment_id,status,detail,source) VALUES($1,'unknown','Imported from shipping email (not carrier-verified)','email')",id)
  if e!=nil{writeError(w,500,"Could not save package history");return}
  created++
 }
 if err=tx.Commit(r.Context());err!=nil{writeError(w,500,"Could not finish email import");return}
 writeJSON(w,200,map[string]any{"created":created,"skipped":skipped})
}
func (s *server) importPackageEmail(w http.ResponseWriter,r *http.Request){
 if !s.requirePackages(w,r){return}
 if !packageCanWrite(r){writeError(w,403,"You cannot import packages");return}
 var in struct{RawEmail string `json:"raw_email"`}
 if decode(r,&in)!=nil{writeError(w,400,"Provide a shipping email");return}
 s.ingestPackageEmail(w,r,currentActor(r).ID,in.RawEmail)
}

func packageTokenHash(raw string)string{
 sum:=sha256.Sum256([]byte(raw));return hex.EncodeToString(sum[:])
}
func (s *server) packageEmailTokenInfo(w http.ResponseWriter,r *http.Request){
 if !s.requirePackages(w,r){return}
 if !packageCanWrite(r){writeError(w,403,"Not permitted");return}
 var exists bool
 err:=s.db.QueryRow(r.Context(),"SELECT EXISTS(SELECT 1 FROM package_inbound_tokens WHERE owner_id=$1)",currentActor(r).ID).Scan(&exists)
 if err!=nil{writeError(w,500,"Could not check email token");return}
 writeJSON(w,200,map[string]any{"configured":exists})
}
func (s *server) rotatePackageEmailToken(w http.ResponseWriter,r *http.Request){
 if !s.requirePackages(w,r){return}
 if !packageCanWrite(r){writeError(w,403,"Not permitted");return}
 secret:=make([]byte,32)
 if _,err:=rand.Read(secret);err!=nil{writeError(w,500,"Could not generate import token");return}
 token:="cpk_"+base64.RawURLEncoding.EncodeToString(secret)
 _,err:=s.db.Exec(r.Context(),"INSERT INTO package_inbound_tokens(owner_id,token_hash) VALUES($1,$2) ON CONFLICT(owner_id) DO UPDATE SET token_hash=EXCLUDED.token_hash,created_at=now()",currentActor(r).ID,packageTokenHash(token))
 if err!=nil{writeError(w,500,"Could not store import token");return}
 s.audit(r,"update","package_email_token",nil,"Rotated inbound shipping-email token",nil)
 writeJSON(w,200,map[string]any{"token":token})
}
func (s *server) revokePackageEmailToken(w http.ResponseWriter,r *http.Request){
 if !s.requirePackages(w,r){return}
 if !packageCanWrite(r){writeError(w,403,"Not permitted");return}
 _,err:=s.db.Exec(r.Context(),"DELETE FROM package_inbound_tokens WHERE owner_id=$1",currentActor(r).ID)
 if err!=nil{writeError(w,500,"Could not revoke email token");return}
 s.audit(r,"delete","package_email_token",nil,"Revoked inbound shipping-email token",nil)
 w.WriteHeader(http.StatusNoContent)
}
func (s *server) inboundPackageEmail(w http.ResponseWriter,r *http.Request){
 auth:=strings.TrimSpace(r.Header.Get("Authorization"))
 if !strings.HasPrefix(auth,"Bearer cpk_"){writeError(w,401,"Missing shipping-email token");return}
 token:=strings.TrimPrefix(auth,"Bearer ")
 if len(token)!=47{writeError(w,401,"Invalid shipping-email token");return}
 var owner uuid.UUID
 e:=s.db.QueryRow(r.Context(),"SELECT t.owner_id FROM package_inbound_tokens t JOIN users u ON u.id=t.owner_id WHERE t.token_hash=$1 AND u.active=true AND u.role!='restricted' AND COALESCE((SELECT value='true' FROM app_settings WHERE key='packages_enabled'),false)",packageTokenHash(token)).Scan(&owner)
 if e!=nil{writeError(w,401,"Invalid or disabled shipping-email token");return}
 // Raw MIME received from a trusted mail gateway. Do not store original mail,
 // auto-follow URLs, or trust its sender as proof of carrier authenticity.
 r.Body=http.MaxBytesReader(w,r.Body,262144)
 defer r.Body.Close()
 b,e:=io.ReadAll(r.Body)
 if e!=nil{writeError(w,413,"Email must be 256 KB or smaller");return}
 s.ingestPackageEmail(w,r,owner,string(b))
}
