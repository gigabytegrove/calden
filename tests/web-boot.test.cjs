"use strict";
const assert=require("node:assert/strict");
const fs=require("node:fs"),path=require("node:path");
const {test}=require("node:test");
const {JSDOM,VirtualConsole}=require("jsdom");
const html=fs.readFileSync(path.resolve(__dirname,"../web/index.html"),"utf8");
const script=fs.readFileSync(path.resolve(__dirname,"../web/app.js"),"utf8");
const person={id:"00000000-0000-4000-8000-000000000001",display_name:"Test User",username:"test",initials:"TU",role:"admin",active:true};
const settings={household_name:"Test",timezone:"America/New_York",week_start:"sunday",default_view:7};
function fixture(url){
  const errors=[];
  const log=new VirtualConsole();
  log.on("jsdomError",error=>errors.push(error.message));
  const dom=new JSDOM(html,{url,runScripts:"outside-only",pretendToBeVisual:true,virtualConsole:log});
  const w=dom.window;
  w.localStorage.setItem("calden_token","test-token");
  w.confirm=()=>false;
  w.scrollTo=()=>{};
  w.matchMedia=()=>({matches:false,addListener(){},removeListener(){}});
  w.fetch=async (request)=>{
    const name=String(request).split("?")[0];
    const results={
      "/api/setup/status":{needs_setup:false},
      "/api/me":person,
      "/api/settings/general":settings,
      "/api/users":[person],
      "/api/calendars":[{id:"00000000-0000-4000-8000-000000000002",name:"Bills",calendar_type:"bill_pay",can_edit:true,color:"#2266aa",event_count:0}],
      "/api/categories":[],
      "/api/notifications":{items:[],unread:0},
      "/api/events":[],
      "/api/health":{ok:true,version:"1.0.3"}
    };
    const body=Object.hasOwn(results,name)?results[name]:[];
    return {ok:true,status:200,json:async()=>body};
  };
  try{w.eval(script)}catch(error){errors.push(error.stack||String(error))}
  return {dom,w,errors};
}
async function assertBoot(url,page){
  const instance=fixture(url);
  try{
    for(let i=0;i<60&&instance.w.document.getElementById("app").classList.contains("hidden");i++)
      await new Promise(resolve=>setTimeout(resolve,20));
    const {document}=instance.w;
    assert.equal(instance.errors.length,0,instance.errors.join("\n"));
    assert.equal(document.getElementById("app").classList.contains("hidden"),false,
      document.getElementById("boot-status").textContent);
    assert.equal(document.getElementById("page-"+page).classList.contains("hidden"),false);
    assert.equal(document.getElementById("boot-status").classList.contains("hidden"),true);
  }finally{instance.dom.window.close()}
}
test("authenticated browser boot on canonical calendar URL",()=>assertBoot("https://calden.example/calendar?date=2026-10-07&days=7","calendar"));
test("authenticated browser boot on shared bills URL",()=>assertBoot("https://calden.example/bills?month=2026-10","bills"));
test("authenticated browser boot on settings subsection",()=>assertBoot("https://calden.example/settings/calendar","settings"));
