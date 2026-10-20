const test=require("node:test");
const assert=require("node:assert/strict");
const fs=require("node:fs");
const path=require("node:path");
const vm=require("node:vm");
const root=path.resolve(__dirname,"..");
const file=p=>fs.readFileSync(path.join(root,p),"utf8");
test("family routine client parses and loads only when enabled",()=>{
 const js=file("web/routines.js"),app=file("web/app.js"),html=file("web/index.html");
 assert.doesNotThrow(()=>new vm.Script(js,{filename:"web/routines.js"}));
 assert.doesNotThrow(()=>new vm.Script(app,{filename:"web/app.js"}));
 assert.match(html,/src="\/routines\.js\?/);
 for(const id of ["page-routines","routine-form","routine-edit-steps","routine-list","routine-day","settings-routines-enabled"])
   assert.ok(html.includes('id="'+id+'"'),"Missing "+id);
 assert.match(app,/caldenPages=new Set\(\["dashboard","shopping","routines"/);
 assert.match(app,/state\.settings\?\.routines_enabled!==true/);
 assert.match(js,/window\.caldenRoutines=\{render,load,enter/);
});
test("routine API uses authenticated routes with server-side permissions",()=>{
 const api=file("internal/api/api.go"),go=file("internal/api/routines.go");
 for(const endpoint of [
  "GET /api/routines","POST /api/routines","PUT /api/routines/{id}",
  "DELETE /api/routines/{id}","PUT /api/routines/{id}/steps/{step_id}/status"
 ]) assert.ok(api.includes('mux.Handle("'+endpoint+'", s.auth('),"Missing auth for "+endpoint);
 assert.match(go,/canManageChores\(actor.Role\)/);
 assert.match(go,/assigned!=actor.ID/);
 assert.match(go,/routineDue\(days,day\)/);
 assert.match(go,/ON CONFLICT\(routine_id,user_id,step_id,occurrence_date\) DO NOTHING/);
 assert.match(go,/archived=true,updated_at=now\(\)/);
 assert.match(file("internal/store/migrations/031_family_routines.sql"),/family_routine_progress/);
 assert.match(file("internal/api/dashboard_preferences.go"),/"routines": true/);
});
test("shopping navigation uses querySelectorAll, not a single element",()=>{
 const app=file("web/app.js");
 assert.doesNotMatch(app,/\$\("\.shopping-nav"\)\.forEach/);
 assert.match(app,/\$\$\("\.shopping-nav"\)\.forEach/);
 assert.match(app,/\$\$\("\.routines-nav"\)\.forEach/);
});
