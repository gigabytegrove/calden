package api

import "testing"

func TestPersonalNotificationPreferencesValidation(t *testing.T){
 start,end:="22:00","07:00"
 badTime:="25:00"
 equal:="08:00"
 cases:=[]struct{name string;value memberNotificationPreferences;want bool}{
  {"defaults",defaultMemberNotificationPreferences(),true},
  {"all disabled but inbox retained",memberNotificationPreferences{EnabledKinds:[]string{}},true},
  {"valid overnight quiet",memberNotificationPreferences{PushEnabled:true,LiveEnabled:true,
   EnabledKinds:[]string{"event_assigned","event_family"},QuietStart:&start,QuietEnd:&end},true},
  {"advance reminders",memberNotificationPreferences{EnabledKinds:[]string{"care_due","care_advance"}},true},
  {"unknown kind",memberNotificationPreferences{EnabledKinds:[]string{"medical_secret"}},false},
  {"duplicate kind",memberNotificationPreferences{EnabledKinds:[]string{"bill_review","bill_review"}},false},
  {"missing kinds",memberNotificationPreferences{},false},
  {"missing quiet end",memberNotificationPreferences{EnabledKinds:[]string{},QuietStart:&start},false},
  {"invalid quiet clock",memberNotificationPreferences{EnabledKinds:[]string{},QuietStart:&badTime,QuietEnd:&end},false},
  {"equal quiet boundaries",memberNotificationPreferences{EnabledKinds:[]string{},QuietStart:&equal,QuietEnd:&equal},false},
 }
 for _,tc:=range cases{t.Run(tc.name,func(t *testing.T){
  if got:=validateMemberNotificationPreferences(tc.value);got!=tc.want{
   t.Fatalf("valid=%v want=%v",got,tc.want)
  }
 })}
}
