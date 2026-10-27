package api

import (
 "bufio"
 "fmt"
 "net"
 "reflect"
 "strings"
 "testing"
 "time"
)

func TestPackageIMAPQuoteRejectsCommandInjection(t *testing.T){
 for _,value:=range []string{"bad\r\nC999 LOGOUT","zero\x00byte",strings.Repeat("A",1025)}{
  if _,err:=imapQuote(value);err==nil{t.Fatalf("unsafe IMAP login accepted")}
 }
 q,err:=imapQuote(`a"b`)
 if err!=nil||q!=`"a\"b"`{t.Fatalf("double quote not escaped: %q / %v",q,err)}
 back,err:=imapQuote(`a\b`)
 if err!=nil||back!=`"a\\b"`{t.Fatalf("backslash not escaped: %q / %v",back,err)}
}
func TestPackageIMAPUIDValidity(t *testing.T){
 v,err:=parsePackageIMAPUIDValidity([]string{"* FLAGS (\\Seen)","* OK [UIDVALIDITY 1234567] UIDs valid","* 4 EXISTS"})
 if err!=nil||v!=1234567{t.Fatalf("unexpected UIDVALIDITY %d / %v",v,err)}
 for _,lines:=range [][]string{{" * OK ready"},{"* OK [UIDVALIDITY 0] ready"},{"* OK [UIDVALIDITY 9999999999999999]"}}{
  if _,err:=parsePackageIMAPUIDValidity(lines);err==nil{t.Fatalf("invalid UIDVALIDITY accepted: %#v",lines)}
 }
}
func TestPackageIMAPSearchOrderAndValidation(t *testing.T){
 got,err:=parsePackageIMAPSearch([]string{"* SEARCH 9 3 12 4"})
 if err!=nil||!reflect.DeepEqual(got,[]uint32{3,4,9,12}){t.Fatalf("UID order %v / %v",got,err)}
 for _,line:=range []string{"* SEARCH 0","* SEARCH -2","* SEARCH x","* SEARCH 4294967296"}{
  if _,err:=parsePackageIMAPSearch([]string{line});err==nil{t.Fatalf("bad search accepted: %s",line)}
 }
}
func TestPackageIMAPLiteralFetch(t *testing.T){
 clientSide,serverSide:=net.Pipe()
 defer clientSide.Close()
 defer serverSide.Close()
 _=clientSide.SetDeadline(time.Now().Add(3*time.Second))
 _=serverSide.SetDeadline(time.Now().Add(3*time.Second))
 done:=make(chan error,1)
 go func(){
  r:=bufio.NewReader(serverSide)
  command,err:=r.ReadString('\n')
  if err!=nil{done<-err;return}
  if !strings.Contains(command,"UID FETCH 55 (BODY.PEEK[])"){done<-fmt.Errorf("unexpected command %q",command);return}
  raw:="From: Test <test@example.com>\r\nSubject: UPS\r\n\r\n1Z999AA10123456784"
  _,err=fmt.Fprintf(serverSide,"* 1 FETCH (UID 55 BODY[] {%d}\r\n%s\r\n)\r\nC000001 OK fetch done\r\n",len(raw),raw)
  done<-err
 }()
 c:=&packageIMAPSession{conn:clientSide,reader:bufio.NewReaderSize(clientSide,packageIMAPMaxLine)}
 _,body,err:=c.command("UID FETCH 55 (BODY.PEEK[])")
 if err!=nil||!strings.Contains(string(body),"1Z999AA10123456784"){t.Fatalf("literal not read safely: %q / %v",body,err)}
 if err:=<-done;err!=nil{t.Fatal(err)}
}
func TestPackageIMAPServerAllowlist(t *testing.T){
 t.Setenv("CALDEN_PACKAGE_IMAP_ALLOWED_HOSTS","imap.example.net, LOCALMAIL.EXAMPLE")
 for _,host:=range []string{"imap.gmail.com","imap.example.net","localmail.example"}{
  if !allowedPackageIMAPHost(host){t.Fatalf("authorized IMAP server rejected: %s",host)}
 }
 for _,host:=range []string{"localhost","127.0.0.1","outlook.office365.com","imap.gmail.com.evil.example"}{
  if allowedPackageIMAPHost(host){t.Fatalf("unsafe IMAP server allowed: %s",host)}
 }
}
