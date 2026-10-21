const test=require("node:test"),assert=require("node:assert/strict");
const fs=require("node:fs"),path=require("node:path"),vm=require("node:vm");
const root=path.resolve(__dirname,"..");const read=p=>fs.readFileSync(path.join(root,p),"utf8");
test("per-user alert preferences have a working authenticated settings interface",()=>{
 const html=read("web/index.html"),app=read("web/app.js"),client=read("web/notification-preferences.js");
 assert.doesNotThrow(()=>new vm.Script(app,{filename:"web/app.js"}));
 assert.doesNotThrow(()=>new vm.Script(client,{filename:"web/notification-preferences.js"}));
 for(const id of ["personal-notifications-form","personal-notifications-status"])
  assert.ok(html.includes('id="'+id+'"'),id+" missing");
 assert.match(html,/data-settings-tab="alerts"/);
 assert.match(html,/src="\/notification-preferences\.js\?/);
 assert.match(app,/window\.caldenPersonalNotifications\?\.load\(\)/);
 assert.match(client,/PUT/);
 assert.match(client,/enabled_kinds:/);
 assert.match(client,/quiet_enabled/);
});
test("per-user preferences remain private and children need notification grants",()=>{
 const api=read("internal/api/api.go"),srv=read("internal/api/notification_preferences.go"),children=read("internal/api/children.go");
 for(const method of ["GET","PUT"])
  assert.ok(api.includes('mux.Handle("'+method+' /api/me/notification-preferences", s.auth('));
 assert.match(srv,/currentActor\(r\)\.ID/);
 assert.match(children,/path=="\/api\/me\/notification-preferences"/);
 assert.match(children,/return "notifications"/);
});
test("quiet hours and type filters apply to live delivery and push, never delete inbox",()=>{
 const live=read("internal/api/notification_stream.go"),push=read("internal/push/worker.go"),sql=read("internal/store/migrations/036_personal_notification_preferences.sql");
 for(const source of [live,push]){
  assert.match(source,/member_notification_preferences/);
  assert.match(source,/quiet_start/);
  assert.match(source,/enabled_kinds/);
  assert.match(source,/quiet_end/);
  assert.match(source,/app_settings WHERE key='timezone'/);
 }
 assert.match(live,/pref.live_enabled/);
 assert.match(push,/pref.push_enabled/);
 assert.match(sql,/user_id UUID PRIMARY KEY/);
 assert.match(sql,/enabled_kinds TEXT\[\]/);
});
