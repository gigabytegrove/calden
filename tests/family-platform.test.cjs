const test=require("node:test");
const assert=require("node:assert/strict");
const fs=require("node:fs");
const path=require("node:path");
const vm=require("node:vm");

const root=path.join(__dirname,"..");
const read=p=>fs.readFileSync(path.join(root,p),"utf8");

test("family web application parses and renders connected addressable pages",()=>{
  const js=read("web/app.js"),html=read("web/index.html");
  assert.doesNotThrow(()=>new vm.Script(js,{filename:"web/app.js"}));
  for(const id of ["page-dashboard","page-shopping","dashboard-customize","dashboard-widgets","shopping-list-picker","shopping-item-form","shopping-item-list","settings-shopping-enabled"]){
    assert.match(html,new RegExp('id="'+id+'"'),"Missing "+id);
  }
  assert.match(html,/data-page="dashboard" href="\/dashboard"/);
  assert.match(html,/data-page="shopping" href="\/shopping"/);
  assert.match(js,/const caldenPages=new Set\(\["dashboard","shopping"/);
  assert.match(js,/window\.caldenFamily\?\.enter\(page\)/);
  assert.match(js,/state\.dashboard=await api\("\/api\/me\/dashboard"\)/);
});

test("personal dashboard endpoints remain authenticated",()=>{
  const code=read("internal/api/api.go");
  for(const method of ["GET","PUT"]){
    assert.ok(code.includes('mux.Handle("'+method+' /api/me/dashboard", s.auth('));
  }
  assert.match(read("internal/api/dashboard_preferences.go"),/currentActor\(r\)\.ID/);
});

test("shopping endpoints, ownership, and household feature controls are wired",()=>{
  const api=read("internal/api/api.go");
  for(const endpoint of [
    "GET /api/shopping/lists","POST /api/shopping/lists",
    "PUT /api/shopping/lists/{id}","DELETE /api/shopping/lists/{id}",
    "GET /api/shopping/lists/{id}/items","POST /api/shopping/lists/{id}/items",
    "PUT /api/shopping/lists/{id}/items/{item_id}",
    "DELETE /api/shopping/lists/{id}/items/{item_id}"
  ])assert.ok(api.includes('mux.Handle("'+endpoint+'", s.auth('),"Missing authenticated "+endpoint);
  const code=read("internal/api/shopping.go"), settings=read("internal/api/settings.go");
  assert.match(code,/created_by=\$2 OR shared=true/);
  assert.match(code,/WHERE id=\$1 AND created_by=\$2/);
  assert.match(code,/requireWritableShoppingList/);
  assert.match(code,/Role!="restricted"/);
  assert.match(settings,/shopping_enabled/);
  assert.match(read("internal/store/migrations/030_household_shopping.sql"),/ON CONFLICT\(key\) DO NOTHING/);
});

test("supply metadata, assignment, change history and feature-gated UI are connected",()=>{
  const js=read("web/app.js"),html=read("web/index.html"),go=read("internal/api/shopping.go");
  const sql=read("internal/store/migrations/039_shopping_supplies_history.sql");
  const routes=read("internal/api/api.go"),child=read("internal/api/children.go");
  assert.doesNotThrow(()=>new vm.Script(js,{filename:"web/app.js"}));
  for(const name of ["category","assigned_to","stock_on_hand","reorder_at"])
    assert.match(html,new RegExp('name="'+name+'"'));
  for(const id of ["shopping-history-toggle","shopping-history-list","shopping-category-filter"])
    assert.ok(html.includes('id="'+id+'"'));
  assert.match(js,/function renderShoppingHistory/);
  assert.match(js,/item\.low_stock/);
  assert.match(js,/function populateShoppingAssignees/);
  assert.match(js,/const visibleItems=state\.shopping\.items\.filter/);
  assert.match(go,/low_stock":lowStock/);
  assert.match(js,/function populateShoppingAssignees/);
  assert.match(go,/CASE WHEN \$13 THEN \$14::numeric ELSE stock_on_hand END/);
  assert.match(go,/allowedShoppingAssignee/);
  assert.match(go,/unrecognized shopping field/);
  assert.match(go,/shoppingListAccess\(w,r,false\)/);
  assert.match(routes,/GET \/api\/shopping\/lists\/\{id\}\/history/);
  assert.match(child,/Suffix\(path,"\/history"\)/);
  assert.match(sql,/household_shopping_history/);
  assert.match(sql,/AFTER INSERT OR UPDATE ON household_shopping_items/);
  assert.match(go,/WITH deleted AS/);
  assert.match(go,/FROM deleted RETURNING item_id/);
  assert.match(sql,/ON DELETE CASCADE/);
});
