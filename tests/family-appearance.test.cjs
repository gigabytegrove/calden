const test=require("node:test"),assert=require("node:assert/strict");
const fs=require("node:fs"),path=require("node:path"),vm=require("node:vm");
const root=path.resolve(__dirname,"..");
const read=p=>fs.readFileSync(path.join(root,p),"utf8");
test("household appearance is a working settings form and client",()=>{
 const html=read("web/index.html"),js=read("web/family-appearance.js"),app=read("web/app.js");
 assert.doesNotThrow(()=>new vm.Script(js,{filename:"web/family-appearance.js"}));
 assert.doesNotThrow(()=>new vm.Script(app,{filename:"web/app.js"}));
 for(const id of ["family-appearance-form","family-appearance-modules","family-appearance-reset","family-appearance-status"]){
  assert.ok(html.includes('id="'+id+'"'),"missing "+id);
 }
 assert.match(html,/src="\/family-appearance\.js\?/);
 assert.match(app,/window\.caldenAppearance\?\.load\(\)/);
 assert.match(app,/window\.caldenAppearance\?\.label\(page/);
});
test("module customization cannot change canonical IDs or bypass permission checks",()=>{
 const api=read("internal/api/api.go"),server=read("internal/api/family_appearance.go"),
       child=read("internal/api/children.go"),sql=read("internal/store/migrations/035_household_presentation.sql");
 assert.match(api,/GET \/api\/family\/appearance/);
 assert.match(api,/PUT \/api\/family\/appearance", s\.auth\(s\.admin\(/);
 assert.match(child,/"GET \/api\/family\/appearance":true/);
 assert.match(server,/approvedPresentationModules/);
 assert.match(server,/appearanceIcons/);
 assert.match(server,/themePresets/);
 assert.match(server,/ON CONFLICT\(module_id\) DO UPDATE/);
 assert.match(sql,/module_id TEXT PRIMARY KEY/);
 assert.match(sql,/family_theme_preset/);
});
test("custom icons are fixed local SVG and labels are escaped",()=>{
 const client=read("web/family-appearance.js");
 assert.match(client,/const shapes=\{/);
 assert.ok(client.includes('title.textContent=module.module_id==="chores"&&module.label==="Chores"'));
 assert.ok(client.includes('defaultChoreName:module.label'));
 assert.match(client,/escapeAttr\(m\.label\)/);
 assert.doesNotMatch(client,/eval\(/);
});
