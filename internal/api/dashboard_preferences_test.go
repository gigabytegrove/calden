package api

import "testing"

func TestDashboardPreferenceValidation(t *testing.T) {
    cases:=[]struct{name string;in dashboardPreferences;want bool}{
        {"default",defaultDashboardPreferences(),true},
        {"simple",dashboardPreferences{"dark","simple",[]string{"calendar","chores"}},true},
        {"empty widgets",dashboardPreferences{"light","standard",[]string{}},true},
        {"invalid appearance",dashboardPreferences{"auto","standard",[]string{"calendar"}},false},
        {"invalid layout",dashboardPreferences{"system","admin",[]string{"calendar"}},false},
        {"unknown widget",dashboardPreferences{"system","standard",[]string{"unreleased"}},false},
        {"duplicate",dashboardPreferences{"system","standard",[]string{"calendar","calendar"}},false},
    }
    for _,tc:=range cases {t.Run(tc.name,func(t *testing.T){
        if got:=validateDashboardPreferences(tc.in);got!=tc.want {t.Fatalf("valid=%v want=%v",got,tc.want)}
    })}
}
