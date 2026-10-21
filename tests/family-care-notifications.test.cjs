const test=require("node:test"),assert=require("node:assert/strict"),fs=require("node:fs"),path=require("node:path");
const root=path.resolve(__dirname,"..");const read=p=>fs.readFileSync(path.join(root,p),"utf8");
test("care reminder maintenance is integrated into existing worker",()=>{
 const m=read("internal/api/notification_maintenance.go"),c=read("internal/api/care_notifications.go");
 assert.match(m,/generateCareNotifications\(scoped,db\)/);
 assert.match(c,/careOccurrenceValid\(x.starts,day,x.repeat\)/);
 assert.match(c,/family_care_occurrences/);
 assert.match(c,/kind='care_due'/);
 assert.match(c,/family_child_guardians/);
 assert.match(c,/ON CONFLICT \(user_id,care_item_id,kind,occurrence_start\)/);
});
test("care notification ACL is rechecked at read and delivery time",()=>{
 const migration=read("internal/store/migrations/037_family_care_reminders.sql");
 assert.match(migration,/calden_can_view_care_notification/);
 assert.match(migration,/u.is_child=false/);
 assert.match(migration,/s.archived=false/);
 assert.match(migration,/family_child_guardians/);
 for(const file of ["internal/api/notifications.go","internal/api/notification_stream.go","internal/push/worker.go"]){
  assert.match(read(file),/calden_can_view_care_notification\(n.care_item_id,n.user_id\)/,file);
 }
});
test("care opt-in is connected to personal notification preferences",()=>{
 const prefs=read("internal/api/notification_preferences.go");
 const html=read("web/index.html"),migration=read("internal/store/migrations/037_family_care_reminders.sql");
 assert.match(prefs,/"care_due":true/);
 assert.match(prefs,/"care_due"/);
 assert.match(html,/name="kind" value="care_due"/);
 assert.match(migration,/notification_care_occurrence_once/);
 assert.match(migration,/enabled_kinds/);
});
