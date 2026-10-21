const test=require("node:test");
const assert=require("node:assert/strict");
const fs=require("node:fs");
const path=require("node:path");
const vm=require("node:vm");
const root=path.resolve(__dirname,"..");
const read=p=>fs.readFileSync(path.join(root,p),"utf8");

test("school, maintenance and pets are addressable, usable family pages",()=>{
 const html=read("web/index.html"),app=read("web/app.js"),client=read("web/family-care.js");
 assert.doesNotThrow(()=>new vm.Script(app,{filename:"app.js"}));
 assert.doesNotThrow(()=>new vm.Script(client,{filename:"family-care.js"}));
 for(const module of ["school","maintenance","pets"]){
  assert.ok(html.includes('id="page-'+module+'"'),module+" page");
  assert.ok(html.includes('data-page="'+module+'" href="/'+module+'"'),module+" route");
  assert.ok(html.includes('id="settings-'+module+'-enabled"'),module+" switch");
  assert.ok(client.includes('"/api/family/care/"+kind'),"API base");
 }
 assert.match(html,/src="\/family-care\.js\?/);
 assert.match(app,/window\.caldenCare\?\.enter\(page\)/);
 assert.match(app,/window\.caldenCare\?\.renderNav\(\)/);
});
test("family care APIs authenticate every operation",()=>{
 const api=read("internal/api/api.go");
 for(const op of [
 "GET /api/family/care/{kind}",
 "POST /api/family/care/{kind}",
 "PUT /api/family/care/{kind}/{id}",
 "GET /api/family/care/{kind}/{id}/items",
 "POST /api/family/care/{kind}/{id}/items",
 "PUT /api/family/care/{kind}/{id}/items/{item_id}",
 "PUT /api/family/care/{kind}/{id}/items/{item_id}/occurrences/{date}",
 "GET /api/family/care/{kind}/{id}/history",
 "GET /api/family/care/{kind}/{id}/terms",
 "POST /api/family/care/{kind}/{id}/terms",
 ])assert.ok(api.includes('mux.Handle("'+op+'", s.auth('),op);
});
test("school records are restricted to current guardians and administrators",()=>{
 const go=read("internal/api/family_care.go");
 assert.match(go,/FROM family_child_guardians WHERE child_id=\$1 AND guardian_id=\$2/);
 assert.match(go,/s\.child_user_id IS NOT NULL AND EXISTS/);
 assert.match(go,/if kind=="school" \{/);
 assert.match(go,/A former guardian must immediately lose access/);
 assert.match(go,/if currentActor\(r\)\.IsChild/);
 assert.match(go,/school.*maintenance.*pets/s);
});
test("care history is preserved and modules default to off",()=>{
 const sql=read("internal/store/migrations/034_family_care.sql"),go=read("internal/api/family_care.go");
 assert.match(sql,/CREATE TABLE family_care_occurrences/);
 assert.match(sql,/PRIMARY KEY\(item_id,occurrence_date\)/);
 assert.match(sql,/\('school_enabled','false'\)/);
 assert.match(sql,/\('maintenance_enabled','false'\)/);
 assert.match(sql,/\('pets_enabled','false'\)/);
 assert.match(go,/careOccurrenceValid\(first,date,rule\)/);
 assert.match(go,/Archived records cannot accept new completions/);
 assert.match(go,/ON CONFLICT\(item_id,occurrence_date\) DO UPDATE/);
 assert.ok(go.includes("archived=$6"));
});
