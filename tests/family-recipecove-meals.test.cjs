const test=require("node:test"),assert=require("node:assert/strict"),fs=require("node:fs"),path=require("node:path"),vm=require("node:vm");
const root=path.resolve(__dirname,"..");const file=p=>fs.readFileSync(path.join(root,p),"utf8");
test("native Calden meal route and script are valid and navigable",()=>{
 const html=file("web/index.html"),app=file("web/app.js"),meal=file("web/meals.js"),api=file("internal/api/api.go");
 assert.doesNotThrow(()=>new vm.Script(meal,{filename:"meals.js"}));
 assert.doesNotThrow(()=>new vm.Script(app,{filename:"app.js"}));
 for(const id of ["page-meals","meals-form","meals-schedule","meals-status","meals-refresh","meals-start"])
  assert.ok(html.includes('id="'+id+'"'),"missing "+id);
 assert.match(html,/data-page="meals" href="\/meals"/);
 assert.match(html,/src="\/meals\.js\?/);
 assert.match(app,/window\.caldenMeals\?\.enter\(page\)/);
 assert.match(app,/page==="meals"&&\(state\.me\?\.is_child/);
 for(const route of ["GET /api/integrations/recipecove/status",
 "GET /api/integrations/recipecove/recipes",
 "GET /api/integrations/recipecove/meals",
 "POST /api/integrations/recipecove/meals",
 "PUT /api/integrations/recipecove/meals/{id}",
 "DELETE /api/integrations/recipecove/meals/{id}"]){
  assert.ok(api.includes('mux.Handle("'+route+'", s.auth('),route);
 }
});
test("RecipeCove secrets are confined to the Go service",()=>{
 const proxy=file("internal/api/recipecove_meals.go"),ui=file("web/meals.js"),html=file("web/index.html");
 assert.match(proxy,/RECIPECOVE_SYNC_KEY/);
 assert.match(proxy,/CheckRedirect/);
 assert.match(proxy,/X-Forknook-API-Key/);
 assert.match(proxy,/RecipeCove requires HTTPS/);
 assert.match(proxy,/a\.IsChild\|\|a\.Role=="restricted"/);
 assert.doesNotMatch(ui,/X-Forknook-API-Key|process\\.env/);
 assert.doesNotMatch(html,/X-Forknook-API-Key/);
 assert.match(ui,/form\.dataset\.requestId/);
 assert.match(ui,/getRandomValues/);
});

test("meal planner protects stale responses and multiple simultaneous writes",()=>{
 const ui=file("web/meals.js"),html=file("web/index.html");
 for(const id of ["meals-previous","meals-next","meals-range-title"])
  assert.ok(html.includes('id="'+id+'"'),"missing navigation "+id);
 assert.match(ui,/if\(run!==revision\)return/);
 assert.match(ui,/if\(busy\)return/);
 assert.match(ui,/\+\+revision;setBusy\(true\)/);
 assert.match(ui,/schedule\.replaceChildren\(\)/);
 assert.match(ui,/document\.createElement\("article"\)/);
 assert.match(ui,/new Option\(/);
 assert.doesNotMatch(ui,/schedule\.innerHTML=/);
 assert.doesNotThrow(()=>new vm.Script(ui,{filename:"meals.js"}));
});
