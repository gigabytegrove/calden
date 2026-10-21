package api

import (
 "net/http"
 "sort"
 "strings"
)

type familyPresentation struct {
 ModuleID string `json:"module_id"`
 Label string `json:"label"`
 Icon string `json:"icon"`
}
type familyAppearancePayload struct {
 Theme string `json:"theme"`
 Modules []familyPresentation `json:"modules"`
}
var approvedPresentationModules=map[string]familyPresentation{
 "dashboard":{ModuleID:"dashboard",Label:"My home",Icon:"house"},
 "calendar":{ModuleID:"calendar",Label:"Calendar",Icon:"calendar"},
 "agenda":{ModuleID:"agenda",Label:"Agenda",Icon:"clipboard-list"},
 "bills":{ModuleID:"bills",Label:"Bill Pay",Icon:"wallet"},
 "tasks":{ModuleID:"tasks",Label:"Tasks",Icon:"list-check"},
 "chores":{ModuleID:"chores",Label:"Chores",Icon:"list-check"},
 "shopping":{ModuleID:"shopping",Label:"Shopping",Icon:"shopping-cart"},
 "routines":{ModuleID:"routines",Label:"Routines",Icon:"list-check"},
 "rewards":{ModuleID:"rewards",Label:"Rewards & Allowances",Icon:"gift"},
 "school":{ModuleID:"school",Label:"School",Icon:"graduation-cap"},
 "maintenance":{ModuleID:"maintenance",Label:"Home upkeep",Icon:"wrench"},
 "pets":{ModuleID:"pets",Label:"Pets",Icon:"paw-print"},
 "notifications":{ModuleID:"notifications",Label:"Notifications",Icon:"bell"},
}
var appearanceIcons=map[string]bool{
 "grid":true,"calendar":true,"house":true,"shopping-cart":true,
 "list-check":true,"gift":true,"graduation-cap":true,"wrench":true,
 "paw-print":true,"bell":true,"wallet":true,"users":true,
 "utensils":true,"heart":true,"clipboard-list":true,
}
var themePresets=map[string]bool{
 "classic":true,"ocean":true,"forest":true,"sunset":true,"charcoal":true,
}
func normalizeFamilyAppearance(in familyAppearancePayload)(familyAppearancePayload,bool){
 if !themePresets[in.Theme]||len(in.Modules)!=len(approvedPresentationModules){return in,false}
 seen:=map[string]bool{}
 for i:=range in.Modules{
  m:=&in.Modules[i]
  m.Label=strings.TrimSpace(m.Label)
  if _,ok:=approvedPresentationModules[m.ModuleID];!ok||seen[m.ModuleID]||len(m.Label)<1||len(m.Label)>32||!appearanceIcons[m.Icon]{
   return in,false
  }
  // Display labels are plain text; do not allow control characters.
  for _,r:=range m.Label{if r<32||r==127{return in,false}}
  seen[m.ModuleID]=true
 }
 return in,true
}
func (s *server) loadFamilyAppearance(w http.ResponseWriter,r *http.Request){
 result:=familyAppearancePayload{Theme:"classic",Modules:[]familyPresentation{}}
 _=s.db.QueryRow(r.Context(),`SELECT COALESCE((SELECT value FROM app_settings
  WHERE key='family_theme_preset'),'classic')`).Scan(&result.Theme)
 if !themePresets[result.Theme]{result.Theme="classic"}
 modules:=map[string]familyPresentation{}
 for key,value:=range approvedPresentationModules{modules[key]=value}
 rows,err:=s.db.Query(r.Context(),`SELECT module_id,display_label,icon
  FROM family_module_presentation`)
 if err!=nil{writeError(w,500,"Could not load family appearance");return}
 for rows.Next(){
  var id,label,icon string
  if err=rows.Scan(&id,&label,&icon);err!=nil{break}
  if base,ok:=modules[id];ok {
   if len(label)>0&&len(label)<=32&&appearanceIcons[icon]{
    base.Label=label;base.Icon=icon;modules[id]=base
   }
  }
 }
 if err==nil{err=rows.Err()};rows.Close()
 if err!=nil{writeError(w,500,"Could not read family appearance");return}
 names:=make([]string,0,len(modules))
 for name:=range modules{names=append(names,name)}
 sort.Strings(names)
 for _,name:=range names{result.Modules=append(result.Modules,modules[name])}
 writeJSON(w,200,result)
}
func (s *server) saveFamilyAppearance(w http.ResponseWriter,r *http.Request){
 var input familyAppearancePayload
 if decode(r,&input)!=nil{writeError(w,400,"Check theme and names");return}
 clean,valid:=normalizeFamilyAppearance(input)
 if !valid{writeError(w,400,"Choose a supported theme, icon and a name for every feature");return}
 tx,err:=s.db.Begin(r.Context());if err!=nil{writeError(w,500,"Could not save family appearance");return}
 defer tx.Rollback(r.Context())
 _,err=tx.Exec(r.Context(),`INSERT INTO app_settings(key,value)
  VALUES('family_theme_preset',$1)
  ON CONFLICT(key) DO UPDATE SET value=EXCLUDED.value,updated_at=now()`,clean.Theme)
 if err!=nil{writeError(w,500,"Could not save theme");return}
 for _,m:=range clean.Modules{
  _,err=tx.Exec(r.Context(),`INSERT INTO family_module_presentation
   (module_id,display_label,icon,updated_by)
   VALUES($1,$2,$3,$4)
   ON CONFLICT(module_id) DO UPDATE SET display_label=EXCLUDED.display_label,
    icon=EXCLUDED.icon,updated_by=EXCLUDED.updated_by,updated_at=now()`,
    m.ModuleID,m.Label,m.Icon,currentActor(r).ID)
  if err!=nil{writeError(w,500,"Could not save module names");return}
 }
 if err=tx.Commit(r.Context());err!=nil{writeError(w,500,"Could not apply family appearance");return}
 s.audit(r,"update","settings",nil,"Updated family theme, names and icons",map[string]any{"theme":clean.Theme})
 writeJSON(w,200,clean)
}
