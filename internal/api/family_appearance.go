package api

import (
 "encoding/json"
 "net/http"
 "strings"
)
type familyAppearance struct {
 Preset string `json:"preset"`
 Labels map[string]string `json:"labels"`
 Icons map[string]string `json:"icons"`
}
var familyPresets=map[string]bool{"ocean":true,"forest":true,"violet":true,"sunset":true}
var familyIcons=map[string]bool{"shopping-cart":true,"school":true,"house":true,"paw-print":true,"list-checks":true,"calendar":true,"star":true,"heart":true,"clipboard-check":true}

func defaultFamilyAppearance() familyAppearance {return familyAppearance{Preset:"ocean",Labels:map[string]string{},Icons:map[string]string{}}}
func validateFamilyAppearance(a familyAppearance)bool{
 if !familyPresets[a.Preset]||len(a.Labels)>len(familySections)||len(a.Icons)>len(familySections){return false}
 for k,v:=range a.Labels {if _,ok:=familySections[k];!ok||len(strings.TrimSpace(v))==0||len(v)>40{return false}}
 for k,v:=range a.Icons {if _,ok:=familySections[k];!ok||!familyIcons[v]{return false}}
 return true
}
func (s *server) getFamilyAppearance(w http.ResponseWriter,r *http.Request){
 out:=defaultFamilyAppearance()
 var stored string
 if err:=s.db.QueryRow(r.Context(),`SELECT COALESCE((SELECT value FROM app_settings WHERE key='family_appearance'),'')`).Scan(&stored);err!=nil{
  writeError(w,500,"Could not load appearance");return
 }
 if stored!="" {
  if err:=json.Unmarshal([]byte(stored),&out);err!=nil || !validateFamilyAppearance(out){
   writeError(w,500,"Could not read household appearance");return
  }
 }
 writeJSON(w,200,out)
}

func (s *server) saveFamilyAppearance(w http.ResponseWriter,r *http.Request){
 var a familyAppearance
 if decode(r,&a)!=nil||!validateFamilyAppearance(a){writeError(w,400,"Check your household theme and feature names");return}
 for k,v:=range a.Labels{a.Labels[k]=strings.TrimSpace(v)}
 raw,err:=json.Marshal(a)
 if err!=nil{writeError(w,500,"Could not save household appearance");return}
 _,err=s.db.Exec(r.Context(),`INSERT INTO app_settings(key,value) VALUES('family_appearance',$1)
 ON CONFLICT(key) DO UPDATE SET value=EXCLUDED.value,updated_at=now()`,string(raw))
 if err!=nil{writeError(w,500,"Could not save household appearance");return}
 s.audit(r,"update","settings",nil,"Changed household theme and labels",nil)
 writeJSON(w,200,a)
}
