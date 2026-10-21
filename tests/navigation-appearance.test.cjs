"use strict";
const test=require("node:test");
const assert=require("node:assert/strict");
const fs=require("node:fs");
const path=require("node:path");
const {JSDOM}=require("jsdom");
const root=path.resolve(__dirname,"..");
const read=file=>fs.readFileSync(path.join(root,file),"utf8");
const html=read("web/index.html"),app=read("web/app.js");
const appearance=read("web/family-appearance.js"),css=read("web/app.css");
const begin=app.indexOf("function decorateNavigation(){");
const end=app.indexOf("\nfunction notificationRows(){",begin);
assert.ok(begin>=0&&end>begin,"navigation initializer must exist");
function fixture(){
 const dom=new JSDOM(html,{url:"https://calden.example/notifications",
  runScripts:"outside-only"});
 const w=dom.window;
 w.state={me:{role:"admin"},settings:{tasks_enabled:true,chores_enabled:true},
  familyAppearance:{theme:"ocean",modules:[
    {module_id:"chores",label:"Chores",icon:"list-check"},
    {module_id:"shopping",label:"Groceries",icon:"shopping-cart"},
    {module_id:"notifications",label:"Notifications",icon:"bell"}
  ]}};
 w.caldenFamily={renderDashboard(){}};
 w.eval(app.slice(begin,end)+"\ndecorateNavigation();");
 w.eval(appearance);
 return {dom,w};
}
test("desktop sidebar gives every available route a single icon and title",()=>{
 const {dom,w}=fixture();
 try{
  const links=[...w.document.querySelectorAll(".primary-nav .nav-item")];
  assert.ok(links.length>=16,"unexpectedly missing core navigation routes");
  for(const link of links){
   const key=link.dataset.page;
   assert.equal(link.querySelectorAll(":scope > .nav-icon svg").length,1,key+" icon");
   assert.equal(link.querySelectorAll(":scope > .nav-label").length,1,key+" label");
   assert.equal(link.querySelectorAll(":scope > .family-nav-label").length,0,key+" duplicate");
   assert.ok(link.querySelector(".nav-label").textContent.trim(),key+" empty label");
  }
  for(const key of ["meals","school","maintenance","pets","rewards","routines"]){
   assert.equal(w.document.querySelector('.primary-nav [data-page="'+key+'"] .nav-icon svg')!=null,true,key);
  }
  const notifications=w.document.querySelector('.primary-nav [data-page="notifications"]');
  assert.ok(notifications.querySelector("#notification-nav-badge"));
  assert.equal(notifications.lastElementChild.id,"notification-nav-badge");
 }finally{dom.window.close()}
});
test("household appearance never duplicates labels after repeated application",()=>{
 const {dom,w}=fixture();
 try{
  const chores=w.document.querySelector('.primary-nav [data-page="chores"]');
  const groceries=w.document.querySelector('.primary-nav [data-page="shopping"]');
  const alerts=w.document.querySelector('.primary-nav [data-page="notifications"]');
  for(let count=0;count<4;count++){
   w.caldenAppearance.apply();
   w.eval("decorateNavigation();");
  }
  assert.equal(chores.querySelector(".nav-label").textContent,"Tasks & Chores");
  assert.equal(groceries.querySelector(".nav-label").textContent,"Groceries");
  assert.equal(alerts.lastElementChild.id,"notification-nav-badge");
  for(const link of [chores,groceries,alerts]){
   assert.equal(link.querySelectorAll(".nav-label,.family-nav-label").length,1);
   assert.equal(link.querySelectorAll(".nav-icon").length,1);
  }
  w.state.familyAppearance.modules[0].label="Family jobs";
  w.caldenAppearance.apply();
  assert.equal(chores.querySelector(".nav-label").textContent,"Family jobs");
  assert.equal(chores.querySelectorAll(".nav-label").length,1);
  assert.equal(w.document.body.dataset.familyTheme,"ocean");
 }finally{dom.window.close()}
});
test("light/dark appearance has whole-shell contrast, not just family widget styles",()=>{
 assert.match(app,/document\.body\.classList\.toggle\("calden-family-dark"/);
 for(const selector of [
  "body.calden-family-dark .app-shell",
  "body.calden-family-dark .workspace-topbar",
  "body.calden-family-dark .workspace :is(input,select,textarea)",
  "body.calden-family-dark .workspace .notification-inbox-row.unread",
  "body.calden-family-dark dialog",
  "body.calden-family-dark[data-family-theme]"
 ])assert.ok(css.includes(selector),selector);
 assert.match(css,/color-scheme:dark/);
 assert.match(css,/\.primary-nav \.nav-item \.nav-label/);
 assert.match(css,/\.notification-tools \.button\{white-space:nowrap/);
});
