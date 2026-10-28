"use strict";
const test=require("node:test");
const assert=require("node:assert/strict");
const fs=require("node:fs");
const path=require("node:path");
const {JSDOM}=require("jsdom");
const root=path.resolve(__dirname,"..");
const read=name=>fs.readFileSync(path.join(root,name),"utf8");
const html=read("web/index.html"),script=read("web/packages.js"),styles=read("web/app.css");
const packageID="00000000-0000-4000-8000-000000000094";
const packageData={
 id:packageID,label:"Replacement laptop battery",tracking_number:"1Z999AA10123456784",
 carrier:"ups",status:"in_transit",can_edit:false,shared:true,status_source:"provider",
 expected_date:"",tracking_url:"",last_tracking_check:"2026-10-28T12:00:00Z",
 next_tracking_check:"2026-10-28T15:00:00Z",latest_scan:"Arrived at hub"
};
const ticks=()=>new Promise(resolve=>setTimeout(resolve,0));
function fixture({pages}={}){
 const dom=new JSDOM(html,{url:"https://calden.example/packages",runScripts:"outside-only"});
 const w=dom.window;
 const dialogs=w.document.querySelectorAll("dialog");
 for(const dialog of dialogs){
  dialog.showModal=function(){this.open=true};
  dialog.close=function(){this.open=false;this.dispatchEvent(new w.Event("close"))};
 }
 w.setInterval=()=>0;
 w.confirm=()=>false;
 w.state={settings:{packages_enabled:true},me:{role:"member",is_child:false},currentPage:"packages"};
 w.escapeHTML=value=>String(value).replace(/[&<>"']/g,ch=>({"&":"&amp;","<":"&lt;",">":"&gt;",'"':"&quot;","'":"&#39;"}[ch]));
 w.escapeAttr=w.escapeHTML;
 w.dateInputValue=d=>d.toISOString().slice(0,10);
 w.renderCalendar=()=>{};
 w.navigate=()=>{};
 const requested=[];
 w.api=async uri=>{
  requested.push(uri);
  if(uri==="/api/packages")return {items:[packageData]};
  if(uri.endsWith("/tracker-health"))return {ready:true,browser_ready:true,state:"ready"};
  if(uri.includes("/history?")){
   const offset=Number(new URL(uri,"https://calden.example").searchParams.get("offset")||0);
   const entries=pages||[
    {items:[
      {status:"in_transit",source:"provider",detail:"Arrived at processing hub · Tallahassee, FL",occurred_at:"2026-10-28T11:23:00Z"},
      {status:"unknown",source:"manual",detail:"Package added",occurred_at:"2026-10-26T09:11:00Z"}
    ],has_more:false,next_offset:2}
   ];
   return entries[offset===0?0:1]||{items:[],has_more:false,next_offset:offset};
  }
  throw new Error("Unexpected request: "+uri);
 };
 w.eval(script);
 return {dom,w,requested};
}
test("tracking history is exposed directly from package rows and the editor",async()=>{
 const f=fixture();
 try{
  await f.w.caldenPackages.load();
  const row=f.w.document.querySelector("#packages-list .package-entry");
  assert.ok(row,"list row missing");
  const history=row.querySelector("[data-package-history]");
  assert.ok(history,"first-class tracking history control missing");
  assert.equal(history.textContent,"Tracking history");
  assert.equal(row.querySelector("[data-package-delete]"),null,"read-only shared viewer unexpectedly has delete");
  history.click();
  await ticks();
  const dialog=f.w.document.getElementById("package-history-dialog");
  assert.equal(dialog.open,true,"history dialog did not open");
  assert.equal(dialog.getAttribute("aria-labelledby"),"package-history-title");
  assert.equal(f.w.document.getElementById("package-history-name").textContent,packageData.label);
  assert.equal(f.w.document.querySelectorAll(".package-history-event").length,2);
  assert.match(f.w.document.getElementById("package-history").textContent,/Carrier scan/);
  assert.match(f.w.document.getElementById("package-history").textContent,/Entered in Calden/);
  assert.match(f.w.document.getElementById("package-history").textContent,/Tallahassee, FL/);
  assert.ok(f.requested.some(x=>x.includes("/history?offset=0")));
  f.w.document.getElementById("package-history-close").click();
  assert.equal(dialog.open,false);
 }finally{f.dom.window.close()}
});
test("older pages are reachable without truncating checkpoint history",async()=>{
 const f=fixture({pages:[
  {items:[{status:"in_transit",source:"provider",detail:"Current scan",occurred_at:"2026-10-28T11:00:00Z"}],next_offset:1,has_more:true},
  {items:[{status:"label_created",source:"provider",detail:"Older scan",occurred_at:"2026-10-27T09:00:00Z"}],next_offset:2,has_more:false}
 ]});
 try{
  await f.w.caldenPackages.load();
  f.w.document.querySelector("[data-package-history]").click();
  await ticks();
  assert.equal(f.w.document.querySelectorAll(".package-history-event").length,1);
  const older=f.w.document.getElementById("package-history-older");
  assert.equal(older.classList.contains("hidden"),false,"older activity was hidden");
  older.click();
  await ticks();
  assert.equal(f.w.document.querySelectorAll(".package-history-event").length,2);
  assert.match(f.w.document.getElementById("package-history").textContent,/Older scan/);
  assert.equal(older.classList.contains("hidden"),true,"older button not disabled after final page");
  assert.ok(f.requested.some(x=>x.includes("/history?offset=1")));
 }finally{f.dom.window.close()}
});
test("UI contract shares approved theme tokens, keyboard focus, and mobile layouts",()=>{
 for(const fragment of [
  "--ui-surface:", "--ui-focus:", "body.calden-family-dark{",
  ".package-history-timeline", ".package-history-source",
  ".package-history-dialog", "@media(max-width:620px)", "prefers-reduced-motion:reduce"
 ])assert.ok(styles.includes(fragment),"missing "+fragment);
 assert.match(styles,/body:not\(\.calden-family-dark\) \.sidebar\{/);
 assert.match(styles,/body\.calden-family-dark \.workspace-topbar\{/);
 assert.match(html,/aria-labelledby="package-history-title"/);
 assert.match(script,/function openPackageHistory\(id\)/);
 assert.match(script,/data-package-history/);
});
