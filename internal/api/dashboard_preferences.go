package api

import (
    "encoding/json"
    "net/http"
    "errors"

    "github.com/jackc/pgx/v5"
)

type dashboardPreferences struct {
    Appearance string   `json:"appearance"`
    Layout     string   `json:"layout"`
    Widgets    []string `json:"widgets"`
}

var dashboardWidgets = map[string]bool{
    "calendar": true, "notifications": true, "shopping": true, "routines": true, "rewards": true, "tasks": true,
    "chores": true, "bills": true, "agenda": true,
}

func childDashboardAllowed(widget string,modules []string)bool{
    switch widget {
    case "calendar","agenda":return childModuleAllowed(modules,"calendar")
    case "chores","tasks":return childModuleAllowed(modules,widget)
    case "routines","rewards","shopping":return childModuleAllowed(modules,widget)
    case "notifications":return true
    default:return false // bills, and future widgets, are never child-permitted
    }
}
func sanitizeChildDashboard(p dashboardPreferences,a actor) dashboardPreferences{
    if !a.IsChild{return p}
    p.Layout=a.ChildLayout
    filtered:=[]string{}
    for _,w:=range p.Widgets{if childDashboardAllowed(w,a.ChildModules){filtered=append(filtered,w)}}
    if len(filtered)==0{filtered=[]string{"notifications"}}
    p.Widgets=filtered
    return p
}
func defaultDashboardPreferences() dashboardPreferences {
    return dashboardPreferences{Appearance:"system",Layout:"standard",Widgets:[]string{"calendar","notifications"}}
}

func validateDashboardPreferences(in dashboardPreferences) bool {
    if in.Appearance!="system" && in.Appearance!="light" && in.Appearance!="dark" {return false}
    if in.Layout!="standard" && in.Layout!="simple" {return false}
    if len(in.Widgets)>len(dashboardWidgets) {return false}
    seen:=make(map[string]bool,len(in.Widgets))
    for _,widget:=range in.Widgets {
        if !dashboardWidgets[widget] || seen[widget] {return false}
        seen[widget]=true
    }
    return true
}

// A person's dashboard is private: never accept a user ID from the client.
func (s *server) getDashboardPreferences(w http.ResponseWriter,r *http.Request) {
    out:=defaultDashboardPreferences()
    var raw []byte
    err:=s.db.QueryRow(r.Context(),
        "SELECT appearance,layout,widgets FROM member_dashboard_preferences WHERE user_id=$1",
        currentActor(r).ID).Scan(&out.Appearance,&out.Layout,&raw)
    if errors.Is(err,pgx.ErrNoRows) {
        writeJSON(w,200,sanitizeChildDashboard(defaultDashboardPreferences(),currentActor(r)));return
    }
    if err!=nil {writeError(w,500,"Could not load your dashboard");return}
    if json.Unmarshal(raw,&out.Widgets)!=nil || !validateDashboardPreferences(out) {
        writeError(w,500,"Could not load your dashboard");return
    }
    writeJSON(w,200,sanitizeChildDashboard(out,currentActor(r)))
}

func (s *server) saveDashboardPreferences(w http.ResponseWriter,r *http.Request) {
    var in dashboardPreferences
    if decode(r,&in)!=nil || !validateDashboardPreferences(in) {
        writeError(w,400,"Choose a valid appearance, layout and widgets");return
    }
    a:=currentActor(r)
    if a.IsChild {
      if in.Layout!=a.ChildLayout {writeError(w,403,"A guardian controls your layout");return}
      for _,widget:=range in.Widgets{
        if !childDashboardAllowed(widget,a.ChildModules){
          writeError(w,403,"This dashboard item is not enabled for your account");return
        }
      }
    }
    raw,err:=json.Marshal(in.Widgets)
    if err!=nil {writeError(w,500,"Could not save your dashboard");return}
    _,err=s.db.Exec(r.Context(),`
        INSERT INTO member_dashboard_preferences(user_id,appearance,layout,widgets)
        VALUES($1,$2,$3,$4::jsonb)
        ON CONFLICT(user_id) DO UPDATE SET
            appearance=EXCLUDED.appearance, layout=EXCLUDED.layout,
            widgets=EXCLUDED.widgets,updated_at=now()`,
        currentActor(r).ID,in.Appearance,in.Layout,string(raw))
    if err!=nil {writeError(w,500,"Could not save your dashboard");return}
    s.audit(r,"update","settings",nil,"Updated personal dashboard preferences",nil)
    writeJSON(w,200,in)
}
