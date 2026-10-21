package api

import (
 "net/http"
 "regexp"

 "github.com/jackc/pgx/v5"
)

type memberNotificationPreferences struct{
 PushEnabled bool `json:"push_enabled"`
 LiveEnabled bool `json:"live_enabled"`
 EnabledKinds []string `json:"enabled_kinds"`
 QuietStart *string `json:"quiet_start"`
 QuietEnd *string `json:"quiet_end"`
}
var allowedNotificationKinds=map[string]bool{
 "event_assigned":true,"event_family":true,"bill_review":true,
}
var clockTime=regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)
func validateMemberNotificationPreferences(in memberNotificationPreferences)bool{
 if in.EnabledKinds==nil||len(in.EnabledKinds)>len(allowedNotificationKinds){return false}
 seen:=map[string]bool{}
 for _,kind:=range in.EnabledKinds{
  if !allowedNotificationKinds[kind]||seen[kind]{return false};seen[kind]=true
 }
 if (in.QuietStart==nil)!=(in.QuietEnd==nil){return false}
 if in.QuietStart!=nil{
  if !clockTime.MatchString(*in.QuietStart)||!clockTime.MatchString(*in.QuietEnd)||*in.QuietStart==*in.QuietEnd{return false}
 }
 return true
}
func defaultMemberNotificationPreferences()memberNotificationPreferences{
 return memberNotificationPreferences{
  PushEnabled:true,LiveEnabled:true,
  EnabledKinds:[]string{"event_assigned","event_family","bill_review"},
 }
}
func (s *server) getPersonalNotificationPreferences(w http.ResponseWriter,r *http.Request){
 out:=defaultMemberNotificationPreferences()
 err:=s.db.QueryRow(r.Context(),`SELECT push_enabled,live_enabled,
  enabled_kinds,quiet_start,quiet_end FROM member_notification_preferences
  WHERE user_id=$1`,currentActor(r).ID).Scan(
  &out.PushEnabled,&out.LiveEnabled,&out.EnabledKinds,&out.QuietStart,&out.QuietEnd)
 if err!=nil{
  // Missing row is the backwards-compatible default; errors must not mask DB failures.
  if err==pgx.ErrNoRows{writeJSON(w,200,out);return}
  writeError(w,500,"Could not load notification preferences");return
 }
 writeJSON(w,200,out)
}
func (s *server) savePersonalNotificationPreferences(w http.ResponseWriter,r *http.Request){
 var in memberNotificationPreferences
 if decode(r,&in)!=nil||!validateMemberNotificationPreferences(in){
  writeError(w,400,"Choose valid delivery settings, notification types and quiet hours");return
 }
 _,err:=s.db.Exec(r.Context(),`INSERT INTO member_notification_preferences
  (user_id,push_enabled,live_enabled,enabled_kinds,quiet_start,quiet_end)
  VALUES($1,$2,$3,$4,$5,$6)
  ON CONFLICT(user_id) DO UPDATE SET push_enabled=EXCLUDED.push_enabled,
   live_enabled=EXCLUDED.live_enabled,enabled_kinds=EXCLUDED.enabled_kinds,
   quiet_start=EXCLUDED.quiet_start,quiet_end=EXCLUDED.quiet_end,updated_at=now()`,
  currentActor(r).ID,in.PushEnabled,in.LiveEnabled,in.EnabledKinds,in.QuietStart,in.QuietEnd)
 if err!=nil{writeError(w,500,"Could not save notification preferences");return}
 s.audit(r,"update","notification_preferences",nil,"Updated personal notification delivery preferences",nil)
 writeJSON(w,200,in)
}
