package api

import (
 "errors"
 "net/http"
 "strings"

 "github.com/google/uuid"
 "github.com/jackc/pgx/v5"
)

var childModuleNames=map[string]bool{
 "calendar":true,"chores":true,"tasks":true,"routines":true,
 "rewards":true,"shopping":true,"notifications":true,"dashboard":true,
}
var childDefaultModules=[]string{"calendar","chores","routines","rewards","notifications"}
type childProfileInput struct{
 DisplayMode string `json:"display_mode"`
 AllowedModules []string `json:"allowed_modules"`
 GuardianIDs []uuid.UUID `json:"guardian_ids"`
}
type createChildInput struct {
 Username string `json:"username"`
 DisplayName string `json:"display_name"`
 Password string `json:"password"`
 childProfileInput
}
func normalizeChildProfile(in childProfileInput)(childProfileInput,bool){
 if in.DisplayMode=="" {in.DisplayMode="simple"}
 if in.DisplayMode!="simple"&&in.DisplayMode!="standard" {return in,false}
 if in.AllowedModules==nil {in.AllowedModules=append([]string(nil),childDefaultModules...)}
 if len(in.AllowedModules)<1||len(in.AllowedModules)>len(childModuleNames) {return in,false}
 seen:=map[string]bool{}
 for _,v:=range in.AllowedModules{
  if !childModuleNames[v]||seen[v]{return in,false};seen[v]=true
 }
 if len(in.GuardianIDs)<1||len(in.GuardianIDs)>12{return in,false}
 guardians:=map[uuid.UUID]bool{}
 for _,v:=range in.GuardianIDs{
  if v==uuid.Nil||guardians[v]{return in,false}
  guardians[v]=true
 }
 return in,true
}
// Child route policy defaults to deny. Module toggles and navigation visibility
// cannot grant an API operation absent from this server allowlist.
func childRequestModule(method,path string)string{
 base:=map[string]bool{
  "GET /api/me":true,"GET /api/users":true,
  "GET /api/settings/general":true,
  "GET /api/family/appearance":true,
  "GET /api/me/dashboard":true,"PUT /api/me/dashboard":true,
  "GET /api/devices":true,"POST /api/devices":true,
  "POST /api/client-errors":true,
 }
 if base[method+" "+path]{return "base"}
 if (method=="GET"||method=="PUT")&&path=="/api/me/notification-preferences"{return "notifications"}
 if (method=="GET"&&(path=="/api/notifications"||path=="/api/notifications/stream"||path=="/api/notifications/push-config"))||(method=="PUT"&&path=="/api/notifications/read-all"){return "notifications"}
 parts:=strings.Split(strings.Trim(path,"/"),"/")
 if len(parts)==4&&parts[0]=="api"&&parts[1]=="notifications"&&parts[3]=="read"&&method=="PUT"{return "notifications"}
 if len(parts)==3&&parts[0]=="api"&&parts[1]=="notifications"&&method=="DELETE"{return "notifications"}
 if len(parts)==3&&parts[0]=="api"&&parts[1]=="devices"&&method=="DELETE"{return "base"}
 if method=="GET"&&(path=="/api/calendars"||path=="/api/categories"||path=="/api/events"){return "calendar"}
 if (method=="GET"&&path=="/api/events/confirmations"){return "calendar"}
 if method=="GET"&&path=="/api/chores"{return "work"}
 if method=="PUT"&&strings.HasPrefix(path,"/api/chores/")&&strings.HasSuffix(path,"/status"){return "work"}
 if method=="GET"&&path=="/api/routines"{return "routines"}
 if method=="PUT"&&strings.HasPrefix(path,"/api/routines/")&&strings.HasSuffix(path,"/status"){return "routines"}
 if method=="GET"&&path=="/api/rewards/overview"{return "rewards"}
 if method=="POST"&&strings.HasPrefix(path,"/api/rewards/catalog/")&&strings.HasSuffix(path,"/claim"){return "rewards"}
 if method=="GET"&&path=="/api/shopping/lists"{return "shopping"}
 if method=="GET"&&strings.HasPrefix(path,"/api/shopping/lists/")&&strings.HasSuffix(path,"/items"){return "shopping"}
 return ""
}
func childModuleAllowed(modules []string,module string)bool{
 if module=="base"{return true}
 if module=="work"{return childModuleAllowed(modules,"chores")||childModuleAllowed(modules,"tasks")}
 for _,m:=range modules{if m==module{return true}}
 return false
}
func (s *server) childProfileForUser(r *http.Request,id uuid.UUID)(childProfileInput,error){
 var profile childProfileInput
 err:=s.db.QueryRow(r.Context(),`SELECT display_mode,allowed_modules
  FROM family_child_profiles WHERE user_id=$1`,id).Scan(&profile.DisplayMode,&profile.AllowedModules)
 return profile,err
}
func (s *server) validGuardians(r *http.Request,ids []uuid.UUID,child uuid.UUID)bool{
 var count int
 err:=s.db.QueryRow(r.Context(),`SELECT count(*) FROM users
  WHERE id=ANY($1::uuid[]) AND active=true AND is_child=false
  AND role IN ('admin','planner','member') AND id<>$2`,ids,child).Scan(&count)
 return err==nil&&count==len(ids)
}
func (s *server) listChildren(w http.ResponseWriter,r *http.Request){
 a:=currentActor(r)
 rows,err:=s.db.Query(r.Context(),`SELECT p.user_id,u.display_name,u.username,u.active,
  p.display_mode,p.allowed_modules
  FROM family_child_profiles p JOIN users u ON u.id=p.user_id
  WHERE $1='admin' OR EXISTS(SELECT 1 FROM family_child_guardians g
     WHERE g.child_id=p.user_id AND g.guardian_id=$2)
  ORDER BY u.display_name`,a.Role,a.ID)
 if err!=nil{writeError(w,500,"Could not load child profiles");return}
 type rec struct{id uuid.UUID;name,username,mode string;active bool;mods []string}
 items:=[]rec{}
 for rows.Next(){
  var p rec
  if err=rows.Scan(&p.id,&p.name,&p.username,&p.active,&p.mode,&p.mods);err!=nil{break}
  items=append(items,p)
 }
 if err==nil{err=rows.Err()};rows.Close()
 if err!=nil{writeError(w,500,"Could not read child profiles");return}
 out:=[]map[string]any{}
 for _,p:=range items{
  guardianRows,e:=s.db.Query(r.Context(),`SELECT guardian_id FROM family_child_guardians
   WHERE child_id=$1 ORDER BY guardian_id`,p.id)
  if e!=nil{writeError(w,500,"Could not load guardians");return}
  guardians:=[]uuid.UUID{}
  for guardianRows.Next(){
   var id uuid.UUID
   if e=guardianRows.Scan(&id);e!=nil{break}
   guardians=append(guardians,id)
  }
  if e==nil{e=guardianRows.Err()};guardianRows.Close()
  if e!=nil{writeError(w,500,"Could not read guardians");return}
  out=append(out,map[string]any{"id":p.id,"display_name":p.name,"username":p.username,
   "active":p.active,"display_mode":p.mode,"allowed_modules":p.mods,
   "guardian_ids":guardians,"can_manage":a.Role=="admin"})
 }
 writeJSON(w,200,map[string]any{"items":out})
}
func (s *server) createChild(w http.ResponseWriter,r *http.Request){
 var in createChildInput
 if decode(r,&in)!=nil{writeError(w,400,"Check the child account details");return}
 in.DisplayName=strings.TrimSpace(in.DisplayName)
 if len(in.DisplayName)<1||len(in.DisplayName)>100{writeError(w,400,"Enter a child profile name");return}
 if err:=validateLogin(in.Username,in.Password);err!=nil{writeError(w,400,err.Error());return}
 var ok bool
 in.childProfileInput,ok=normalizeChildProfile(in.childProfileInput)
 if !ok{writeError(w,400,"Choose guardians, a layout and allowed features");return}
 if !s.validGuardians(r,in.GuardianIDs,uuid.Nil) {
  writeError(w,400,"Choose active adult guardians");return
 }
 hash,err:=hashPassword(in.Password)
 if err!=nil{writeError(w,500,"Could not secure child account");return}
 tx,err:=s.db.Begin(r.Context())
 if err!=nil{writeError(w,500,"Could not create child account");return}
 defer tx.Rollback(r.Context())
 var id uuid.UUID
 err=tx.QueryRow(r.Context(),`INSERT INTO users
  (username,display_name,password_hash,role,initials,is_child)
  VALUES(lower($1),$2,$3,'restricted',$4,true) RETURNING id`,
  strings.TrimSpace(in.Username),in.DisplayName,hash,initials(in.DisplayName)).Scan(&id)
 if err!=nil{writeError(w,409,"That username is already used");return}
 _,err=tx.Exec(r.Context(),`INSERT INTO family_child_profiles
  (user_id,display_mode,allowed_modules) VALUES($1,$2,$3)`,
  id,in.DisplayMode,in.AllowedModules)
 if err!=nil{writeError(w,500,"Could not configure child profile");return}
 for _,guardian:=range in.GuardianIDs{
  if _,err=tx.Exec(r.Context(),`INSERT INTO family_child_guardians(child_id,guardian_id)
    VALUES($1,$2)`,id,guardian);err!=nil{writeError(w,500,"Could not assign guardians");return}
 }
 if err=tx.Commit(r.Context());err!=nil{writeError(w,500,"Could not save child account");return}
 s.audit(r,"create","child",&id,"Created child account",map[string]any{"guardians":len(in.GuardianIDs)})
 writeJSON(w,201,map[string]any{"id":id})
}
func (s *server) updateChild(w http.ResponseWriter,r *http.Request){
 id,err:=uuid.Parse(r.PathValue("id"))
 if err!=nil{writeError(w,400,"Invalid child account");return}
 var in childProfileInput
 if decode(r,&in)!=nil{writeError(w,400,"Check child profile settings");return}
 var ok bool
 in,ok=normalizeChildProfile(in)
 if !ok{writeError(w,400,"Choose guardians, a layout and allowed features");return}
 if !s.validGuardians(r,in.GuardianIDs,id){writeError(w,400,"Choose active adult guardians");return}
 tx,err:=s.db.Begin(r.Context())
 if err!=nil{writeError(w,500,"Could not update child profile");return}
 defer tx.Rollback(r.Context())
 var active bool
 err=tx.QueryRow(r.Context(),`SELECT active FROM users
  WHERE id=$1 AND is_child=true AND role='restricted' FOR UPDATE`,id).Scan(&active)
 if errors.Is(err,pgx.ErrNoRows){writeError(w,404,"Child account not found");return}
 if err!=nil{writeError(w,500,"Could not load child account");return}
 _,err=tx.Exec(r.Context(),`UPDATE family_child_profiles SET display_mode=$2,
  allowed_modules=$3,updated_at=now() WHERE user_id=$1`,
  id,in.DisplayMode,in.AllowedModules)
 if err!=nil{writeError(w,500,"Could not update child settings");return}
 _,err=tx.Exec(r.Context(),"DELETE FROM family_child_guardians WHERE child_id=$1",id)
 if err!=nil{writeError(w,500,"Could not update guardians");return}
 for _,guardian:=range in.GuardianIDs{
  if _,err=tx.Exec(r.Context(),`INSERT INTO family_child_guardians(child_id,guardian_id)
   VALUES($1,$2)`,id,guardian);err!=nil{writeError(w,500,"Could not update guardians");return}
 }
 if err=tx.Commit(r.Context());err!=nil{writeError(w,500,"Could not save child profile");return}
 s.audit(r,"update","child",&id,"Updated child permissions and guardians",nil)
 writeJSON(w,200,map[string]any{"id":id,"active":active})
}
