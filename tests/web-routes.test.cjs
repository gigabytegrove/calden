"use strict";
const test=require("node:test"),assert=require("node:assert/strict"),fs=require("node:fs"),path=require("node:path");
const read=p=>fs.readFileSync(path.join(__dirname,"..",p),"utf8");
const js=read("web/app.js"),html=read("web/index.html"),server=read("internal/api/api.go");
test("page and detail routes are handled",()=>{
  for(const item of ["function caldenPageURL","function caldenEventURL","function readCalDenRoute","async function applyCalDenRoute","popstate","history.pushState"])
    assert.ok(js.includes(item),item);
  assert.ok(js.includes("sameCaldenOccurrence"));
  assert.ok(html.includes('id="event-details-link"'));
});
test("navigation entries are real anchors",()=>{
  for(const page of ["calendar","bills","agenda","people","calendars","categories","notifications","activity","settings"])
    assert.ok(html.includes('href="/'+page+'"'),page);
});
test("all web resources disable caching and stale clients update",()=>{
  assert.ok(server.includes('Surrogate-Control'));
  assert.ok(server.includes('no-store, no-cache, must-revalidate'));
  assert.ok(html.includes('name="calden-version" content="'+read("VERSION").trim()+'"'));
  assert.ok(js.includes("startCalDenVersionWatch"));
  assert.ok(js.includes("checkCalDenClientVersion"));
});

test("permalink navigation has no underline",()=>{assert.ok(read("web/app.css").includes(".primary-nav a.nav-item,.mobile-nav a[data-page]{text-decoration:none!important"));});

test("settings subsections preserve deep links",()=>{
  for(const page of ["integrations","updates","backups"]){
    assert.ok(html.includes('data-settings-tab="'+page+'"'));
    assert.ok(html.includes('data-settings-pane="'+page+'"'));
  }
  assert.ok(js.includes('settingsAliases.has(parts[0])'));
});
