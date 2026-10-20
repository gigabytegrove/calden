const test=require("node:test"),assert=require("node:assert/strict");
const fs=require("node:fs"),path=require("node:path"),vm=require("node:vm");
const root=path.resolve(__dirname,"..");
const file=p=>fs.readFileSync(path.join(root,p),"utf8");

test("child management UI is wired into existing People page",()=>{
 const html=file("web/index.html"),js=file("web/children.js"),app=file("web/app.js");
 assert.doesNotThrow(()=>new vm.Script(js,{filename:"children.js"}));
 assert.doesNotThrow(()=>new vm.Script(app,{filename:"app.js"}));
 for(const id of ["children-form","children-list","children-guardians","children-modules","children-save","children-cancel","children-status"]){
  assert.ok(html.includes('id="'+id+'"'),id+" missing");
 }
 assert.match(html,/src="\/children\.js\?/);
 assert.match(js,/\/api\/family\/children/);
 assert.match(js,/window\.caldenChildren=\{load,render,applyChildView,enter/);
 assert.match(app,/window\.caldenChildren\?\.applyChildView\(\)/);
 assert.match(app,/state\.me\?\.is_child/);
});
test("security restrictions are enforced by server, not child navigation",()=>{
 const api=file("internal/api/api.go"),children=file("internal/api/children.go");
 const resources=file("internal/api/resources.go"),notifications=file("internal/api/notifications.go");
 const prefs=file("internal/api/dashboard_preferences.go");
 for(const route of ["GET /api/family/children","POST /api/family/children","PUT /api/family/children/{id}"]){
  assert.ok(api.includes('mux.Handle("'+route+'", s.auth('),route+" should authenticate");
 }
 assert.match(api,/if isChild \{/);
 assert.match(api,/childRequestModule\(r.Method,r.URL.Path\)/);
 assert.match(api,/childModuleAllowed\(childModules,module\)/);
 assert.match(children,/return ""\s*\}\s*func childModuleAllowed/);
 assert.match(children,/guardian_ids/);
 assert.match(resources,/NOT \$5 OR c.calendar_type<>'bill_pay'/);
 assert.match(resources,/pc.calendar_type<>'bill_pay' AND rc.calendar_type<>'bill_pay'/);
 assert.match(notifications,/NOT \$4 OR c.id IS NULL OR c.calendar_type<>'bill_pay'/);
 assert.match(prefs,/childDashboardAllowed/);
 assert.match(prefs,/A guardian controls your layout/);
});
test("child account migrations preserve adult accounts",()=>{
 const sql=file("internal/store/migrations/033_child_guardian_profiles.sql");
 assert.match(sql,/is_child BOOLEAN NOT NULL DEFAULT FALSE/);
 assert.match(sql,/NOT is_child OR role='restricted'/);
 assert.match(sql,/CREATE TABLE family_child_guardians/);
 assert.match(sql,/CREATE TABLE family_child_profiles/);
 assert.match(file("internal/api/resources.go"),/"is_child": child/);
});
