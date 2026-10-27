package api

import (
 "net/http"
 "strings"
 "time"
 "github.com/google/uuid"
)

type packageInput struct {
 ID string `json:"id"`
 Title string `json:"title"`
 Carrier string `json:"carrier"`
 TrackingNumber string `json:"tracking_number"`
 ExpectedDate string `json:"expected_date"`
 Status string `json:"status"`
 Shared bool `json:"shared"`
 Notes string `json:"notes"`
}
func (s *server) packagesAllowed(w http.ResponseWriter,r *http.Request) bool {
 if currentActor(r).IsChild {writeError(w,403,"Not available for child accounts");return false}
 var enabled bool
 err:=s.db.QueryRow(r.Context(),"SELECT COALESCE((SELECT value='true' FROM app_settings WHERE key='packages_enabled'),false)").Scan(&enabled)
 if err!=nil {writeError(w,500,"Could not load package settings");return false}
 if !enabled {writeError(w,403,"Package tracking is disabled");return false}
 return true
}
func(s *server) listPackages(w http.ResponseWriter,r *http.Request){
 if !s.packagesAllowed(w,r){return}
 rows,err:=s.db.Query(r.Context(),`SELECT id,title,carrier,tracking_number,COALESCE(to_char(expected_date,'YYYY-MM-DD'),''),status,shared,notes FROM packages WHERE owner_id=$1 OR shared=true ORDER BY expected_date ASC NULLS LAST,created_at DESC LIMIT 500`,currentActor(r).ID)
 if err!=nil {writeError(w,500,"Could not list packages");return}
 defer rows.Close()
 out:=[]packageInput{}
 for rows.Next(){
  var p packageInput
  if err:=rows.Scan(&p.ID,&p.Title,&p.Carrier,&p.TrackingNumber,&p.ExpectedDate,&p.Status,&p.Shared,&p.Notes);err!=nil{writeError(w,500,"Could not load package");return}
  out=append(out,p)
 }
 if rows.Err()!=nil{writeError(w,500,"Could not finish loading packages");return}
 writeJSON(w,200,out)
}
func(s *server) savePackage(w http.ResponseWriter,r *http.Request){
 if !s.packagesAllowed(w,r){return}
 var p packageInput
 if decode(r,&p)!=nil{writeError(w,400,"Invalid package data");return}
 p.Title=cleanText(strings.TrimSpace(p.Title),160)
 p.TrackingNumber=strings.TrimSpace(p.TrackingNumber)
 p.Notes=cleanText(strings.TrimSpace(p.Notes),2000)
 if p.Title==""||len(p.TrackingNumber)>120||len(p.Notes)>2000{writeError(w,400,"Check package fields");return}
 switch p.Carrier{case "amazon","usps","ups","fedex","other":default:writeError(w,400,"Invalid carrier");return}
 switch p.Status{case "pending","label_created","in_transit","out_for_delivery","delivered","delayed","exception":default:writeError(w,400,"Invalid package status");return}
 var date any
 if p.ExpectedDate!=""{
  d,err:=time.Parse("2006-01-02",p.ExpectedDate)
  if err!=nil||d.Format("2006-01-02")!=p.ExpectedDate{writeError(w,400,"Invalid delivery date");return}
  date=d
 }
 if r.Method=="POST" {
  err:=s.db.QueryRow(r.Context(),`INSERT INTO packages(owner_id,title,carrier,tracking_number,expected_date,status,shared,notes) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id`,currentActor(r).ID,p.Title,p.Carrier,p.TrackingNumber,date,p.Status,p.Shared,p.Notes).Scan(&p.ID)
  if err!=nil{writeError(w,500,"Could not save package");return}
 } else {
  id,err:=uuid.Parse(r.PathValue("id"))
  if err!=nil{writeError(w,400,"Invalid package ID");return}
  tag,err:=s.db.Exec(r.Context(),`UPDATE packages SET title=$1,carrier=$2,tracking_number=$3,expected_date=$4,status=$5,shared=$6,notes=$7,updated_at=now() WHERE id=$8 AND owner_id=$9`,p.Title,p.Carrier,p.TrackingNumber,date,p.Status,p.Shared,p.Notes,id,currentActor(r).ID)
  if err!=nil{writeError(w,500,"Could not update package");return}
  if tag.RowsAffected()==0{writeError(w,404,"Package unavailable");return}
  p.ID=id.String()
 }
 s.audit(r,"update","package",nil,"Saved package",nil)
 writeJSON(w,200,p)
}
func(s *server) deletePackage(w http.ResponseWriter,r *http.Request){
 if !s.packagesAllowed(w,r){return}
 id,err:=uuid.Parse(r.PathValue("id"))
 if err!=nil{writeError(w,400,"Invalid package ID");return}
 tag,err:=s.db.Exec(r.Context(),"DELETE FROM packages WHERE id=$1 AND owner_id=$2",id,currentActor(r).ID)
 if err!=nil{writeError(w,500,"Could not delete package");return}
 if tag.RowsAffected()==0{writeError(w,404,"Package unavailable");return}
 s.audit(r,"delete","package",nil,"Deleted package",nil)
 writeJSON(w,200,map[string]bool{"deleted":true})
}
