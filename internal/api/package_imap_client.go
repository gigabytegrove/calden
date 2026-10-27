package api

import (
 "bufio"
 "context"
 "crypto/tls"
 "errors"
 "fmt"
 "io"
 "net"
 "regexp"
 "sort"
 "strconv"
 "strings"
 "time"
)

const packageIMAPMaxMessage = 262144
const packageIMAPMaxLine = 32768
const packageIMAPMaxPerPoll = 40

var imapUIDValidityPattern=regexp.MustCompile(`(?i)\[UIDVALIDITY ([0-9]+)\]`)
var imapLiteralPattern=regexp.MustCompile(`\{([0-9]+)\}$`)

type packageIMAPMessage struct {
 UID uint32
 Data []byte
}
type packageIMAPSession struct {
 conn net.Conn
 reader *bufio.Reader
 counter uint32
}
func imapQuote(s string)(string,error){
 if strings.ContainsAny(s,"\r\n\x00")||len(s)>1024{return "",errors.New("invalid IMAP login value")}
 return "\"" + strings.ReplaceAll(strings.ReplaceAll(s,"\\","\\\\"),"\"","\\\"") + "\"",nil
}
func (c *packageIMAPSession) readLine()(string,error){
 b,err:=c.reader.ReadSlice('\n')
 if err!=nil{return "",fmt.Errorf("IMAP response too long or connection closed: %w",err)}
 if len(b)>packageIMAPMaxLine{return "",errors.New("IMAP response line exceeds size limit")}
 return strings.TrimRight(string(b),"\r\n"),nil
}
func (c *packageIMAPSession) command(command string)([]string,[]byte,error){
 c.counter++
 tag:=fmt.Sprintf("C%06d",c.counter)
 if _,err:=io.WriteString(c.conn,tag+" "+command+"\r\n");err!=nil{return nil,nil,err}
 var lines []string
 var body []byte
 for n:=0;n<4096;n++{
  line,err:=c.readLine();if err!=nil{return nil,nil,err}
  if strings.HasPrefix(line,tag+" "){
   fields:=strings.Fields(line)
   if len(fields)<2||!strings.EqualFold(fields[1],"OK"){return nil,nil,errors.New("IMAP command rejected")}
   return lines,body,nil
  }
  lines=append(lines,line)
  if len(lines)>500{return nil,nil,errors.New("IMAP response contains too many lines")}
  match:=imapLiteralPattern.FindStringSubmatch(line)
  if len(match)!=2{continue}
  size,err:=strconv.ParseInt(match[1],10,64)
  if err!=nil||size<0||size>10*1024*1024{return nil,nil,errors.New("IMAP literal exceeds safety limit")}
  if size>packageIMAPMaxMessage{
   if _,err:=io.CopyN(io.Discard,c.reader,size);err!=nil{return nil,nil,err}
   continue
  }
  data:=make([]byte,int(size))
  if _,err:=io.ReadFull(c.reader,data);err!=nil{return nil,nil,err}
  body=data
 }
 return nil,nil,errors.New("IMAP response exceeded command limit")
}
func parsePackageIMAPUIDValidity(lines []string)(uint32,error){
 for _,line:=range lines{
  m:=imapUIDValidityPattern.FindStringSubmatch(line)
  if len(m)==2{
   v,e:=strconv.ParseUint(m[1],10,32)
   if e==nil&&v>0{return uint32(v),nil}
  }
 }
 return 0,errors.New("IMAP server did not provide UIDVALIDITY")
}
func parsePackageIMAPSearch(lines []string)([]uint32,error){
 out:=make([]uint32,0)
 for _,line:=range lines{
  if !strings.HasPrefix(strings.ToUpper(line),"* SEARCH"){continue}
  for _,f:=range strings.Fields(line)[2:]{
   n,e:=strconv.ParseUint(f,10,32)
   if e!=nil||n==0{return nil,errors.New("Invalid IMAP search response")}
   out=append(out,uint32(n))
  }
 }
 sort.Slice(out,func(i,j int)bool{return out[i]<out[j]})
 return out,nil
}
// scanPackageIMAP opens a verified TLS 1.2+ connection and performs only
// EXAMINE, UID SEARCH and UID FETCH BODY.PEEK[] operations. No mail is modified.
// Imported messages are bounded, and receipt acknowledgement uses UIDVALIDITY.
func scanPackageIMAP(ctx context.Context,host,user,secret string,lastValidity,lastUID uint32,consume func(packageIMAPMessage)error)(uint32,uint32,int,error){
 if host==""||user==""||secret==""{return 0,lastUID,0,errors.New("IMAP mailbox is not configured")}
 dialer:=&net.Dialer{Timeout:10*time.Second}
 raw,err:=dialer.DialContext(ctx,"tcp",net.JoinHostPort(host,"993"))
 if err!=nil{return 0,lastUID,0,errors.New("Could not reach secure IMAP server")}
 conn:=tls.Client(raw,&tls.Config{ServerName:host,MinVersion:tls.VersionTLS12})
 defer conn.Close()
 if err=conn.HandshakeContext(ctx);err!=nil{return 0,lastUID,0,errors.New("IMAP TLS verification failed")}
 deadline:=time.Now().Add(45*time.Second)
 if d,ok:=ctx.Deadline();ok&&d.Before(deadline){deadline=d}
 _=conn.SetDeadline(deadline)
 client:=&packageIMAPSession{conn:conn,reader:bufio.NewReaderSize(conn,packageIMAPMaxLine)}
 greeting,err:=client.readLine()
 if err!=nil||!strings.HasPrefix(strings.ToUpper(greeting),"* OK"){return 0,lastUID,0,errors.New("IMAP server refused connection")}
 qUser,err:=imapQuote(user);if err!=nil{return 0,lastUID,0,err}
 qSecret,err:=imapQuote(secret);if err!=nil{return 0,lastUID,0,err}
 if _,_,err=client.command("LOGIN "+qUser+" "+qSecret);err!=nil{return 0,lastUID,0,errors.New("IMAP authentication failed; check app password")}
 lines,_,err:=client.command("EXAMINE \"INBOX\"")
 if err!=nil{return 0,lastUID,0,errors.New("Could not open inbox read-only")}
 validity,err:=parsePackageIMAPUIDValidity(lines)
 if err!=nil{return 0,lastUID,0,err}
 if validity!=lastValidity{lastUID=0}
 date:=time.Now().UTC().AddDate(0,0,-14).Format("02-Jan-2006")
 lines,_,err=client.command("UID SEARCH SINCE "+date)
 if err!=nil{return validity,lastUID,0,errors.New("IMAP search failed")}
 uids,err:=parsePackageIMAPSearch(lines)
 if err!=nil{return validity,lastUID,0,err}
 // Limit the first scan to recent mail. Incremental sync processes messages in
 // ascending UID order, never advancing a cursor over messages we did not fetch.
 if lastUID==0&&len(uids)>packageIMAPMaxPerPoll{uids=uids[len(uids)-packageIMAPMaxPerPoll:]}
 count:=0
 for _,uid:=range uids{
  if uid<=lastUID{continue}
  _,raw,err:=client.command(fmt.Sprintf("UID FETCH %d (BODY.PEEK[])",uid))
  if err!=nil{return validity,lastUID,count,errors.New("IMAP message fetch failed")}
  if len(raw)>0{
   if err=consume(packageIMAPMessage{UID:uid,Data:raw});err!=nil{return validity,lastUID,count,err}
  }
  lastUID=uid
  count++
  if count>=packageIMAPMaxPerPoll{break}
 }
 return validity,lastUID,count,nil
}
