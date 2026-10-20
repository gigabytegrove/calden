package api

import (
 "testing"
 "github.com/google/uuid"
)

func TestChildProfileValidation(t *testing.T){
 adult:=uuid.New()
 cases:=[]struct{name string;in childProfileInput;valid bool}{
  {"default",childProfileInput{GuardianIDs:[]uuid.UUID{adult}},true},
  {"custom",childProfileInput{GuardianIDs:[]uuid.UUID{adult},DisplayMode:"standard",AllowedModules:[]string{"calendar","shopping"}},true},
  {"no guardian",childProfileInput{DisplayMode:"simple",AllowedModules:[]string{"calendar"}},false},
  {"duplicate guardians",childProfileInput{GuardianIDs:[]uuid.UUID{adult,adult}},false},
  {"invalid guardian",childProfileInput{GuardianIDs:[]uuid.UUID{uuid.Nil}},false},
  {"unknown module",childProfileInput{GuardianIDs:[]uuid.UUID{adult},AllowedModules:[]string{"banking"}},false},
  {"bill module",childProfileInput{GuardianIDs:[]uuid.UUID{adult},AllowedModules:[]string{"bills"}},false},
  {"duplicate module",childProfileInput{GuardianIDs:[]uuid.UUID{adult},AllowedModules:[]string{"chores","chores"}},false},
  {"bad layout",childProfileInput{GuardianIDs:[]uuid.UUID{adult},DisplayMode:"admin"},false},
  {"empty modules",childProfileInput{GuardianIDs:[]uuid.UUID{adult},AllowedModules:[]string{}},false},
 }
 for _,tc:=range cases{
  t.Run(tc.name,func(t *testing.T){
   got,ok:=normalizeChildProfile(tc.in)
   if ok!=tc.valid{t.Fatalf("valid=%v wanted=%v",ok,tc.valid)}
   if tc.name=="default"&&(got.DisplayMode!="simple"||len(got.AllowedModules)!=5){
    t.Fatalf("unsafe defaults: %+v",got)
   }
  })
 }
}
func TestChildRouteDefaultDeny(t *testing.T){
 cases:=[]struct{method,path,module string}{
  {"GET","/api/me","base"},
  {"GET","/api/me/dashboard","base"},
  {"PUT","/api/me/dashboard","base"},
  {"GET","/api/users","base"},
  {"GET","/api/notifications","notifications"},
  {"GET","/api/notifications/stream","notifications"},
  {"GET","/api/notifications/push-config","notifications"},
  {"PUT","/api/notifications/read-all","notifications"},
  {"PUT","/api/notifications/123/read","notifications"},
  {"DELETE","/api/notifications/123","notifications"},
  {"GET","/api/events","calendar"},
  {"GET","/api/calendars","calendar"},
  {"GET","/api/chores","work"},
  {"PUT","/api/chores/123/status","work"},
  {"GET","/api/routines","routines"},
  {"PUT","/api/routines/123/steps/456/status","routines"},
  {"GET","/api/rewards/overview","rewards"},
  {"POST","/api/rewards/catalog/123/claim","rewards"},
  {"GET","/api/shopping/lists","shopping"},
  {"GET","/api/shopping/lists/123/items","shopping"},
  {"PUT","/api/settings/general",""},
  {"GET","/api/devices/household",""},
  {"POST","/api/rewards/points/adjust",""},
  {"PUT","/api/bills/123/payment",""},
  {"POST","/api/events",""},
  {"DELETE","/api/calendars/123",""},
  {"POST","/api/shopping/lists",""},
  {"POST","/api/routines",""},
  {"POST","/api/chores",""},
  {"GET","/api/family/children",""},
  {"PUT","/api/family/children/123",""},
  {"POST","/api/family/children",""},
  {"POST","/api/devices/household",""},
  {"GET","/api/rewards/catalog/123/claim",""},
  {"GET","/api/backups",""},
 }
 for _,tc:=range cases{
  t.Run(tc.method+" "+tc.path,func(t *testing.T){
   if m:=childRequestModule(tc.method,tc.path);m!=tc.module{t.Fatalf("got %q expected %q",m,tc.module)}
  })
 }
}
func TestChildPermissionsAndDashboard(t *testing.T){
 mods:=[]string{"calendar","chores","rewards"}
 if childModuleAllowed(mods,"shopping"){t.Fatal("disabled shopping allowed")}
 if !childModuleAllowed(mods,"calendar"){t.Fatal("enabled calendar denied")}
 if !childModuleAllowed(mods,"base"){t.Fatal("base denied")}
 if childDashboardAllowed("bills",mods){t.Fatal("child can view bill widget")}
 if childDashboardAllowed("shopping",mods){t.Fatal("child can view disabled shopping")}
 if childDashboardAllowed("notifications",mods){t.Fatal("child can view disabled notifications")}
 a:=actor{IsChild:true,ChildLayout:"simple",ChildModules:mods}
 in:=dashboardPreferences{Appearance:"dark",Layout:"standard",Widgets:[]string{"calendar","bills","shopping","rewards"}}
 out:=sanitizeChildDashboard(in,a)
 if out.Layout!="simple"{t.Fatalf("guardian layout bypassed: %s",out.Layout)}
 if len(out.Widgets)!=2||out.Widgets[0]!="calendar"||out.Widgets[1]!="rewards"{
  t.Fatalf("disallowed widgets leaked: %#v",out.Widgets)
 }
}

func TestChildTaskOnlyGrant(t *testing.T){
 if !childModuleAllowed([]string{"tasks"},"work"){t.Fatal("tasks-only child cannot enter work module")}
 if !childModuleAllowed([]string{"chores"},"work"){t.Fatal("chores-only child cannot enter work module")}
 if childModuleAllowed([]string{"shopping"},"work"){t.Fatal("unrelated module grants work")}
 if childModuleAllowed([]string{"tasks"},"chores"){t.Fatal("tasks-only child gained chores")}
 if childWorkModule("task")!="tasks"||childWorkModule("chore")!="chores"{
  t.Fatal("incorrect server-side kind mapping")
 }
}
