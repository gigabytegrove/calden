"use strict";
const test=require("node:test"),assert=require("node:assert/strict"),fs=require("node:fs"),path=require("node:path");
const read=p=>fs.readFileSync(path.join(__dirname,"..",p),"utf8");
const js=read("web/app.js"),html=read("web/index.html"),css=read("web/app.css");
test("allocated bill displays a high contrast text presentation symbol",()=>{
  assert.ok(js.includes('icon:"⌛︎"'));
  assert.ok(css.includes('background:#ffca2c;color:#322600'));
});
test("system tools are nested in settings, not global navigation",()=>{
  for(const section of ["updates","integrations","backups"]){
    assert.ok(!html.includes('id="page-'+section+'"'));
    assert.ok(html.includes('data-settings-tab="'+section+'"'));
    assert.ok(html.includes('data-settings-pane="'+section+'"'));
  }
});
test("completed update hides progress and offers optional activity history",()=>{
  assert.ok(html.includes('id="update-history-details"'));
  assert.ok(js.includes('toggle("hidden",!active)'));
  assert.ok(js.includes('else historyDetails.open=false'));
  assert.ok(js.includes("Update completed successfully."));
});
