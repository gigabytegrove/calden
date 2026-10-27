package api

import (
 "io"
 "net/mail"
 "regexp"
 "strings"
 "time"
)

var packageEmailEstimate = regexp.MustCompile(`(?i)\b(?:estimated delivery|expected delivery|delivery date|scheduled delivery|arriving)(?:\s+date)?\s*(?::|is|on|by|for)?\s*(?:(?:mon|tues?|wednes|thurs?|fri|satur|sun)(?:day)?\s*,?\s*)?((?:20\d{2}-[01]\d-[0-3]\d)|(?:(?:jan(?:uary)?|feb(?:ruary)?|mar(?:ch)?|apr(?:il)?|may|jun(?:e)?|jul(?:y)?|aug(?:ust)?|sep(?:tember)?|oct(?:ober)?|nov(?:ember)?|dec(?:ember)?)\.?\s+[0-3]?\d(?:,?\s+20\d{2})?))\b`)

func extractPackageEstimatedDate(raw string) string {
 if len(raw)>packageIMAPMaxMessage{return ""}
 msg,err:=mail.ReadMessage(strings.NewReader(raw));if err!=nil{return ""}
 reference:=time.Now().UTC()
 if d,e:=msg.Header.Date();e==nil{reference=d}
 body:=readPackageMailBody(msg.Header.Get("Content-Type"),msg.Header.Get("Content-Transfer-Encoding"),io.LimitReader(msg.Body,131072))
 text:=msg.Header.Get("Subject")+"\n"+body
 m:=packageEmailEstimate.FindStringSubmatch(text)
 if len(m)<2{return ""}
 value:=strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(m[1]),".",""),"  "," ")
 layouts:=[]string{"2006-01-02","January 2, 2006","Jan 2, 2006","January 2 2006","Jan 2 2006","January 2","Jan 2"}
 var result time.Time
 var parsed bool
 for _,layout:=range layouts{
  d,e:=time.Parse(layout,value)
  if e==nil{
   if !strings.Contains(layout,"2006"){
    d=time.Date(reference.Year(),d.Month(),d.Day(),0,0,0,0,time.UTC)
    if d.Before(reference.AddDate(0,0,-7)){d=d.AddDate(1,0,0)}
   }
   result=d;parsed=true;break
  }
 }
 if !parsed{return ""}
 start:=time.Date(reference.Year(),reference.Month(),reference.Day(),0,0,0,0,time.UTC)
 if result.Before(start.AddDate(0,0,-7))||result.After(start.AddDate(1,0,7)){return ""}
 return result.Format("2006-01-02")
}
