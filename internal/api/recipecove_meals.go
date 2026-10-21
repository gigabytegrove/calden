package api

import (
 "bytes"
 "context"
 "encoding/json"
 "errors"
 "io"
 "net/http"
 "net/url"
 "os"
 "strconv"
 "strings"
 "time"
)

// RecipeCove owns the meal records. Calden never duplicates them in its database.
// Credentials are injected by the deployment, never shared with browser clients.
func recipeCoveConfig() (string,string,error){
 origin:=strings.TrimSpace(os.Getenv("RECIPECOVE_URL"))
 key:=strings.TrimSpace(os.Getenv("RECIPECOVE_SYNC_KEY"))
 if origin==""||key=="" {return "","",errors.New("RecipeCove integration is not configured")}
 u,err:=url.Parse(origin)
 if err!=nil||u.Host==""||u.User!=nil||u.RawQuery!=""||u.Fragment!=""||(u.Path!=""&&u.Path!="/"){
  return "","",errors.New("Invalid RecipeCove origin")
 }
 if u.Scheme!="https" && !(u.Scheme=="http"&&os.Getenv("RECIPECOVE_ALLOW_HTTP")=="true"){
  return "","",errors.New("RecipeCove requires HTTPS unless explicitly allowed on a trusted network")
 }
 if strings.ContainsAny(u.Host,"\\\r\n")||strings.Contains(u.Host,"@"){
  return "","",errors.New("Invalid RecipeCove host")
 }
 if !strings.HasPrefix(key,"fnk_")||len(key)>256{return "","",errors.New("Invalid RecipeCove key")}
 return strings.TrimRight(origin,"/"),key,nil
}
func recipeCoveProxy(w http.ResponseWriter,r *http.Request,method,route string,payload []byte){
 origin,key,err:=recipeCoveConfig()
 if err!=nil {writeError(w,503,err.Error());return}
 ctx,cancel:=context.WithTimeout(r.Context(),9*time.Second)
 defer cancel()
 req,err:=http.NewRequestWithContext(ctx,method,origin+"/api/v2/integrations/calden"+route,bytes.NewReader(payload))
 if err!=nil {writeError(w,500,"Could not prepare RecipeCove request");return}
 req.Header.Set("X-Forknook-API-Key",key)
 req.Header.Set("Accept","application/json")
 if payload!=nil{req.Header.Set("Content-Type","application/json")}
 client:=&http.Client{Timeout:8*time.Second,CheckRedirect:func(*http.Request,[]*http.Request)error{
   return http.ErrUseLastResponse
 }}
 response,err:=client.Do(req)
 if err!=nil {writeError(w,502,"RecipeCove is not reachable");return}
 defer response.Body.Close()
 if response.StatusCode<200||response.StatusCode>=300 {
  switch response.StatusCode{
  case 401,403:writeError(w,502,"RecipeCove rejected the integration key")
  case 404:writeError(w,404,"RecipeCove meal or recipe was not found")
  case 409:writeError(w,409,"RecipeCove request conflicts with an existing meal")
  case 422:writeError(w,400,"RecipeCove rejected the meal details")
  default:writeError(w,502,"RecipeCove could not complete this request")
  }
  return
 }
 // Read one byte beyond the permitted size: a truncated JSON prefix is not a valid response.
 constMax:=int64(2<<20)
 data,err:=io.ReadAll(io.LimitReader(response.Body,constMax+1))
 if err!=nil||int64(len(data))>constMax||!json.Valid(data){
  writeError(w,502,"RecipeCove returned an invalid or oversized response");return
 }
 w.Header().Set("Content-Type","application/json; charset=utf-8")
 w.Header().Set("Cache-Control","no-store")
 w.WriteHeader(response.StatusCode)
 _,_=w.Write(data)
}
type coveMealInput struct{
 Day string `json:"day"`
 Meal string `json:"meal"`
 RecipeID int64 `json:"recipe_id"`
 Servings float64 `json:"servings"`
 Note string `json:"note"`
 RequestID string `json:"request_id,omitempty"`
}
func validCoveMeal(in coveMealInput,create bool)bool{
 if _,err:=time.Parse("2006-01-02",in.Day);err!=nil{return false}
 switch in.Meal{case "anytime","breakfast","lunch","dinner","snack":default:return false}
 if in.RecipeID<1||in.Servings<=0||in.Servings>100||len(in.Note)>500{return false}
 if create{
  if len(in.RequestID)<8||len(in.RequestID)>100{return false}
  for _,r:=range in.RequestID{if !(r>='a'&&r<='z'||r>='A'&&r<='Z'||r>='0'&&r<='9'||r=='-'||r=='_'){return false}}
 }
 return true
}
func allowCoveUser(w http.ResponseWriter,r *http.Request)bool{
 a:=currentActor(r)
 if a.IsChild||a.Role=="restricted"{
  writeError(w,403,"Meal planning is managed by adult household members");return false
 }
 return true
}
func (s *server) coveStatus(w http.ResponseWriter,r *http.Request){
 if !allowCoveUser(w,r){return}
 _,_,err:=recipeCoveConfig()
 writeJSON(w,200,map[string]any{"configured":err==nil,"source":"RecipeCove"})
}
func (s *server) coveRecipes(w http.ResponseWriter,r *http.Request){
 if !allowCoveUser(w,r){return}
 recipeCoveProxy(w,r,"GET","/recipes",nil)
}
func (s *server) coveMeals(w http.ResponseWriter,r *http.Request){
 if !allowCoveUser(w,r){return}
 from,to:=r.URL.Query().Get("start"),r.URL.Query().Get("end")
 d1,e1:=time.Parse("2006-01-02",from)
 d2,e2:=time.Parse("2006-01-02",to)
 if e1!=nil||e2!=nil||d2.Before(d1)||d2.Sub(d1)>92*24*time.Hour{
  writeError(w,400,"Choose a valid range of up to 93 days");return
 }
 recipeCoveProxy(w,r,"GET","/meal-plan?start="+url.QueryEscape(from)+"&end="+url.QueryEscape(to),nil)
}
func (s *server) coveSaveMeal(w http.ResponseWriter,r *http.Request){
 if !allowCoveUser(w,r){return}
 var in coveMealInput
 if decode(r,&in)!=nil||!validCoveMeal(in,r.Method=="POST"){
  writeError(w,400,"Choose a recipe, meal, date and valid servings");return
 }
 route:="/meal-plan"
 if r.Method=="PUT"{
  id,err:=strconv.ParseInt(r.PathValue("id"),10,64)
  if err!=nil||id<1{writeError(w,400,"Invalid meal");return}
  route+="/"+strconv.FormatInt(id,10)
 }
 payload,err:=json.Marshal(in)
 if err!=nil{writeError(w,400,"Could not prepare meal");return}
 recipeCoveProxy(w,r,r.Method,route,payload)
}
func (s *server) coveDeleteMeal(w http.ResponseWriter,r *http.Request){
 if !allowCoveUser(w,r){return}
 id,err:=strconv.ParseInt(r.PathValue("id"),10,64)
 if err!=nil||id<1{writeError(w,400,"Invalid meal");return}
 recipeCoveProxy(w,r,"DELETE","/meal-plan/"+strconv.FormatInt(id,10),nil)
}
