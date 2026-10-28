"use strict";
const test=require("node:test"),assert=require("node:assert/strict"),fs=require("node:fs");
const html=fs.readFileSync("web/index.html","utf8");
const css=fs.readFileSync("web/app.css","utf8");
const packages=fs.readFileSync("web/packages.js","utf8");
test("approved light and dark appearances preserve unified module controls",()=>{
  for(const token of ["--ui-surface","--ui-field","--ui-foreground","--ui-border","--ui-focus","--ui-control-height"])assert.ok(css.includes(token),token);
  assert.match(css,/body\.calden-family-dark\{/);
  assert.match(css,/body:not\(\.calden-family-dark\) \.sidebar\{/);
  for(const module of ["family-widget","family-shopping-row","routine-step","rewards-item","meals-entry","package-entry","family-care-page"])assert.ok(css.includes(module),module);
  assert.match(css,/dialog:is\(\.package-dialog/);
  assert.match(css,/@media\(max-width:620px\)/);
  assert.match(css,/prefers-reduced-motion:reduce/);
});
test("package list exposes activity and a usable history route",()=>{
  assert.match(packages,/data-package-history/);
  assert.match(packages,/View full history/);
  assert.match(packages,/Latest carrier activity:/);
  assert.match(packages,/No verified carrier scans recorded yet/);
  assert.match(packages,/openPackageHistory\(history\.dataset\.packageHistory\)/);
  assert.match(packages,/\/api\/packages\/.*?\/history\?offset=/);
  assert.match(html,/id="package-history-dialog"/);
  assert.match(html,/id="package-history-older"/);
  assert.match(html,/id="package-history-refresh"/);
  assert.match(css,/\.package-history-dialog \.package-history-event/);
});
test("tracking history cannot disguise manual updates as verified carrier scans",()=>{
 assert.match(packages,/source==="provider"\?"Carrier scan"/);
 assert.match(packages,/source==="manual"\?"Entered in Calden"/);
 assert.match(packages,/No tracking activity recorded yet/);
 const backend=fs.readFileSync("internal/api/packages.go","utf8");
 assert.match(backend,/func \(s \*server\) packageHistory\(/);
 assert.match(backend,/LIMIT 101 OFFSET \$2/);
 assert.match(backend,/packageByID\(w,r,false\)/);
});
