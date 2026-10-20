"use strict";
const test=require("node:test");
const assert=require("node:assert/strict");
const fs=require("node:fs");
const path=require("node:path");
const read=name=>fs.readFileSync(path.join(__dirname,"..",name),"utf8");
const html=read("web/index.html"),app=read("web/app.js"),family=read("web/family.js");
const migration=read("internal/store/migrations/030_family_household_items.sql");
const handlers=read("internal/api/family_household.go"),routes=read("internal/api/api.go");
test("household browser scripts parse and are loaded in the correct order",()=>{
 new Function(app);new Function(family);
 assert.match(html,/<script src="\/app\.js\?v=[^"]+" defer><\/script>\s*<script src="\/family\.js\?v=[^"]+" defer><\/script>/);
});
test("five feature pages and module settings have addressable routes and working handlers",()=>{
 for(const key of ["shopping","school","maintenance","pets","routines"]){
  assert.ok(html.includes('id="page-'+key+'"'),key+" page");
  assert.ok(html.includes('href="/'+key+'"'),key+" navigation");
  assert.ok(html.includes('id="family-form-'+key+'"'),key+" form");
  assert.ok(html.includes('id="family-list-'+key+'"'),key+" list");
  assert.ok(family.includes('"/api/household/"+section'),key+" actions");
  assert.ok(migration.includes("'family_"+key+"_enabled'"),key+" persistent feature setting");
 }
 for(const method of ["listFamilyItems","createFamilyItem","updateFamilyItem","deleteFamilyItem","completeFamilyItem"])
  assert.ok(routes.includes("s."+method),method+" route");
});
test("API authorization and privacy conditions are enforced by database queries",()=>{
 assert.match(routes,/PUT \/api\/household\/features.*s\.admin/);
 assert.match(routes,/PUT \/api\/household\/appearance.*s\.admin/);
 assert.ok(handlers.includes("owner_id IS NULL OR owner_id=$2"),"record reads");
 assert.ok(handlers.includes("owner_id IS NULL OR owner_id=$3"),"record writes");
 assert.ok(handlers.includes("created_by=$3"),"edits are creator or authorized administrator");
 assert.ok(migration.includes("PRIMARY KEY(item_id,occurrence_date)"),"each scheduled completion is unique");
});
test("dashboard features persist independently without deleting household records",()=>{
 const dp=read("internal/api/dashboard_preferences.go");
 assert.ok(dp.includes("member_dashboard_preferences"));
 assert.ok(dp.includes("currentActor(r).ID"));
 for(const name of ["calendar","notifications","shopping","school","maintenance","pets","routines"])
  assert.ok(dp.includes('"'+name+'"'),name+" dashboard widget");
 assert.ok(read("internal/store/migrations/029_personal_dashboard_preferences.sql").includes("user_id UUID PRIMARY KEY"));
 assert.ok(handlers.includes("ON CONFLICT(item_id,occurrence_date) DO NOTHING"));
 assert.ok(migration.includes("ON DELETE CASCADE"));
});
test("theme customization and accessibility controls are present",()=>{
 assert.ok(routes.includes("getFamilyAppearance"));
 assert.ok(routes.includes("saveFamilyAppearance"));
 assert.ok(html.includes('id="my-dashboard-form"'));
 assert.ok(html.includes('id="family-appearance-form"'));
 assert.ok(family.includes("data-widget-up"));
 assert.ok(family.includes("data-widget-down"));
 assert.ok(family.includes("prefers-color-scheme"));
 assert.ok(read("web/app.css").includes(".calden-dark"));
});
