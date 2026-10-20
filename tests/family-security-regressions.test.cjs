const test=require("node:test");
const assert=require("node:assert/strict");
const fs=require("node:fs");
const path=require("node:path");
const vm=require("node:vm");
const root=path.resolve(__dirname,"..");
const read=p=>fs.readFileSync(path.join(root,p),"utf8");

test("rewards, routines, and family client scripts are valid JavaScript",()=>{
 for(const name of ["web/app.js","web/routines.js","web/rewards.js","web/children.js"]){
  assert.doesNotThrow(()=>new vm.Script(read(name),{filename:name}),name);
 }
});

test("navigation uses multi-element selectors for optional family modules",()=>{
 const app=read("web/app.js");
 assert.doesNotMatch(app,/(^|[^$])\\$\\("\\.(shopping|routines|rewards)-nav"\\)\\.forEach/m);
 for(const module of ["shopping","routines","rewards"]){
  assert.ok(app.includes('$(".'+module+'-nav").forEach'),module);
 }
});

test("child access to tasks or chores is mutually constrained by configured kinds",()=>{
 const children=read("internal/api/children.go"),chores=read("internal/api/chores.go");
 assert.match(children,/module=="work"/);
 assert.match(children,/childModuleAllowed\(modules,"tasks"\)/);
 assert.match(children,/childModuleAllowed\(modules,"chores"\)/);
 assert.match(chores,/childWorkModule\(ch.Kind\)/);
 assert.match(chores,/childWorkModule\(kind\)/);
 assert.match(chores,/if currentActor\(r\)\.IsChild \{/);
 assert.match(chores,/assigned=own/);
});

test("all child notification transports enforce permissions and exclude bill content",()=>{
 const children=read("internal/api/children.go"),rest=read("internal/api/notifications.go"),
  live=read("internal/api/notification_stream.go"),push=read("internal/push/worker.go"),
  maint=read("internal/api/notification_maintenance.go"),dash=read("internal/api/dashboard_preferences.go");
 assert.match(children,/return "notifications"/);
 assert.match(rest,/n.kind<>'bill_review'/);
 assert.match(rest,/(?:\$5 OR c.id IS NULL)/);
 assert.match(rest,/childModuleAllowed\(a.ChildModules,"calendar"\)/);
 assert.match(live,/liveChild,liveRole/);
 assert.match(live,/'calendar'=ANY\(cp.allowed_modules\)/);
 assert.match(live,/'notifications'=ANY\(cp.allowed_modules\)/);
 assert.match(live,/n.kind<>'bill_review'/);
 assert.match(push,/usr.active=true/);
 assert.match(push,/d.user_id=n.user_id/);
 assert.match(push,/'notifications'=ANY\(child.allowed_modules\)/);
 assert.match(push,/'calendar'=ANY\(child.allowed_modules\)/);
 assert.match(push,/cal.calendar_type<>'bill_pay'/);
 assert.match(maint,/is_child=false/);
 assert.match(dash,/case "notifications":return childModuleAllowed\(modules,"notifications"\)/);
});

test("guardian permission revocation hides child UI modules",()=>{
 const app=read("web/app.js"),children=read("web/children.js");
 assert.match(app,/allowed.has\("notifications"\)/);
 assert.match(children,/notifications:modules.has\("notifications"\)/);
 assert.match(app,/window\.caldenChildren\?\.applyChildView\(\)/);
 assert.match(children,/window\.caldenChildren=\{load,render,applyChildView,enter/);
});
