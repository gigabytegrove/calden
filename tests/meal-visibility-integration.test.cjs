"use strict";
const test=require("node:test");
const assert=require("node:assert/strict");
const fs=require("node:fs");
const path=require("node:path");
const root=path.resolve(__dirname,"..");
const read=filename=>fs.readFileSync(path.join(root,filename),"utf8");

test("Meal Planning has an actual persisted household toggle with a safe default",()=>{
 const settings=read("internal/api/settings.go"),html=read("web/index.html");
 assert.ok(settings.includes('MealsEnabled bool `json:"meals_enabled"`'));
 assert.ok(settings.includes('MealsEnabled *bool `json:"meals_enabled"`'));
 assert.match(settings,/MealsEnabled: true/);
 assert.match(settings,/"meals_enabled",in.MealsEnabled/);
 assert.match(settings,/out.MealsEnabled = value != "false"/);
 assert.match(html,/id="settings-meals-enabled"/);
 assert.match(html,/Enable Meal Planning/);
});

test("Meal Planning is blocked at every adult API handler when globally disabled",()=>{
 const source=read("internal/api/recipecove_meals.go");
 assert.match(source,/func \(s \*server\) coveEnabled\(ctx context.Context\)/);
 assert.match(source,/Meal Planning is disabled for this household/);
 assert.equal((source.match(/if !s.allowCoveUser\(w,r\)\{return\}/g)||[]).length,4);
 assert.match(source,/SELECT COALESCE\(/);
 assert.match(source,/value <> 'false'/);
 assert.match(source,/"enabled":enabled/);
 assert.doesNotMatch(source,/writeJSON\(w,200,map\[string\]any\{[^}]*key/);
});

test("browser navigation and route honor disabled Meal Planning",()=>{
 const ui=read("web/app.js"),html=read("web/index.html"),module=read("web/meals.js");
 assert.match(html,/class="nav-item meals-nav" data-page="meals"/);
 assert.equal(ui.split('$$(".meals-nav").forEach').length-1,2);
 assert.match(ui,/if\(page==="meals"&&\(state.settings\?\.meals_enabled===false/);
 assert.match(ui,/name==="meals"&&state.settings.meals_enabled===false/);
 assert.match(ui,/loadRecipeCoveIntegrationStatus\(\)/);
 assert.match(module,/page!=="meals"\|\|state.settings\?\.meals_enabled===false/);
});

test("RecipeCove has honest status in Integrations, with secrets confined to server",()=>{
 const html=read("web/index.html"),app=read("web/app.js"),api=read("internal/api/recipecove_meals.go");
 const begin=html.indexOf('id="recipecove-title"');
 const calendar=html.indexOf('class="panel integration-card google-migration-card"');
 const monita=html.indexOf('class="panel integration-card monita-integration-card"');
 assert.ok(begin>=0&&calendar>begin&&monita>calendar,"integration cards in coherent sections");
 for(const id of ["recipecove-connection-badge","recipecove-integration-status","recipecove-check","recipecove-feature-link"])
  assert.ok(html.includes('id="'+id+'"'),"Missing UI control "+id);
 assert.match(app,/data.enabled===false\?"Disabled"/);
 assert.match(app,/not yet verified/);
 assert.match(api,/RECIPECOVE_SYNC_KEY/);
 assert.match(html,/credentials stay on the server/i);
 assert.doesNotMatch(app,/RECIPECOVE_SYNC_KEY\s*:/);
});
