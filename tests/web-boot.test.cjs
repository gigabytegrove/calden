"use strict";
const assert=require("node:assert/strict");
const fs=require("node:fs"),path=require("node:path");
const {test}=require("node:test");
const {JSDOM,VirtualConsole}=require("jsdom");
const html=fs.readFileSync(path.resolve(__dirname,"../web/index.html"),"utf8");
const script=fs.readFileSync(path.resolve(__dirname,"../web/app.js"),"utf8");
const person={id:"00000000-0000-4000-8000-000000000001",display_name:"Test User",username:"test",initials:"TU",role:"admin",active:true};
const settings={household_name:"Test",timezone:"America/New_York",week_start:"sunday",default_view:7};
function fixture(url,options={}){
  const errors=[];
  const log=new VirtualConsole();
  log.on("jsdomError",error=>errors.push(error.message));
  const dom=new JSDOM(html,{url,runScripts:"outside-only",pretendToBeVisual:true,virtualConsole:log});
  const w=dom.window;
  w.localStorage.setItem("calden_token","test-token");
  w.confirm=()=>false;
  w.scrollTo=()=>{};
  w.matchMedia=()=>({matches:options.systemDark===true,addListener(){},removeListener(){},addEventListener(){}});
  let dashboard={appearance:"system",layout:"standard",widgets:["calendar","notifications"]};
  const saves=[];
  let householdSettings={...settings,meals_enabled:options.mealsEnabled!==false};
  const featureSaves=[];
  for(const form of w.document.forms){
    for(const element of form.elements){
      if(element.name && !(element.name in form))
        Object.defineProperty(form,element.name,{configurable:true,value:form.elements.namedItem(element.name)});
    }
  }
  w.fetch=async (request,fetchOptions={})=>{
    const name=String(request).split("?")[0];
    if(name==="/api/me/dashboard"&&fetchOptions.method==="PUT"){
      const next=JSON.parse(fetchOptions.body);
      if(options.failSave)return {ok:false,status:503,json:async()=>({error:"Simulated save failure"})};
      dashboard=next;saves.push(next);
      return {ok:true,status:200,json:async()=>dashboard};
    }
    if(name==="/api/settings/features"&&fetchOptions.method==="PUT"){
      const next=JSON.parse(fetchOptions.body);
      householdSettings={...householdSettings,...next};
      featureSaves.push(next);
      return {ok:true,status:200,json:async()=>householdSettings};
    }
    const results={
      "/api/setup/status":{needs_setup:false},
      "/api/me":person,
      "/api/me/dashboard":dashboard,
      "/api/settings/general":householdSettings,
      "/api/integrations/recipecove/status":{configured:false,enabled:householdSettings.meals_enabled},
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
  return {dom,w,errors,saves,featureSaves,dashboard:()=>dashboard,householdSettings:()=>householdSettings};
}
async function assertBoot(url,page){
  const instance=fixture(url);
  try{
    for(let i=0;i<100&&instance.w.document.getElementById("boot-status").classList.contains("hidden")===false;i++)
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

test("no single element querySelector forEach calls",()=>{
  const broken=script.split("\n").filter(line=>/^\s*\$\("[^"]+"\)\.forEach/.test(line));
  assert.deepEqual(broken,[]);
});

test("runtime does not call an undefined triple-dollar selector helper",()=>{
  assert.doesNotMatch(script,/\$\$\$\(/);
  assert.match(script,/\$\$\("\.rewards-nav"\)\.forEach/);
});

test("global Settings appearance can override system Dark and persists without touching dashboard widgets",async()=>{
  const instance=fixture("https://calden.example/settings/calendar",{systemDark:true});
  try{
    const w=instance.w,document=w.document;
    for(let i=0;i<120&&!document.getElementById("boot-status").classList.contains("hidden");i++)
      await new Promise(resolve=>setTimeout(resolve,20));
    assert.equal(instance.errors.length,0,instance.errors.join("\n"));
    const theme=document.getElementById("settings-appearance");
    assert.ok(theme,"global Appearance setting must exist");
    assert.equal(theme.value,"system");
    assert.equal(document.body.classList.contains("calden-family-dark"),true,
      "Follow my device should respect system dark preference");
    theme.value="light";
    theme.dispatchEvent(new w.Event("change",{bubbles:true}));
    for(let i=0;i<120&&theme.disabled;i++)
      await new Promise(resolve=>setTimeout(resolve,20));
    assert.equal(instance.saves.length,1);
    assert.equal(instance.saves[0].appearance,"light");
    assert.deepEqual(instance.saves[0].widgets,["calendar","notifications"]);
    assert.equal(instance.saves[0].layout,"standard");
    assert.equal(document.body.classList.contains("calden-family-dark"),false);
    assert.equal(document.getElementById("dashboard-appearance").value,"light");
    assert.equal(document.getElementById("settings-appearance").value,"light");
    theme.value="dark";theme.dispatchEvent(new w.Event("change",{bubbles:true}));
    for(let i=0;i<120&&theme.disabled;i++)
      await new Promise(resolve=>setTimeout(resolve,20));
    assert.equal(instance.saves.length,2);
    assert.equal(document.body.classList.contains("calden-family-dark"),true);
    theme.value="system";theme.dispatchEvent(new w.Event("change",{bubbles:true}));
    for(let i=0;i<120&&theme.disabled;i++)
      await new Promise(resolve=>setTimeout(resolve,20));
    assert.equal(instance.saves.length,3);
    assert.equal(document.body.classList.contains("calden-family-dark"),true);
  }finally{instance.dom.window.close()}
});
test("failed theme save restores the original appearance and gives feedback",async()=>{
  const instance=fixture("https://calden.example/settings/calendar",{systemDark:true,failSave:true});
  try{
    const w=instance.w,document=w.document;
    for(let i=0;i<120&&!document.getElementById("boot-status").classList.contains("hidden");i++)
      await new Promise(resolve=>setTimeout(resolve,20));
    const theme=document.getElementById("settings-appearance");
    theme.value="light";theme.dispatchEvent(new w.Event("change",{bubbles:true}));
    for(let i=0;i<120&&theme.disabled;i++)
      await new Promise(resolve=>setTimeout(resolve,20));
    assert.equal(theme.value,"system");
    assert.equal(document.body.classList.contains("calden-family-dark"),true);
    assert.match(document.getElementById("settings-appearance-status").textContent,/Unable to save/);
    assert.equal(instance.saves.length,0);
  }finally{instance.dom.window.close()}
});

test("disabled Meal Planning cannot be opened directly and its navigation is hidden",async()=>{
 const instance=fixture("https://calden.example/meals",{mealsEnabled:false});
 try{
  const {w}=instance;
  for(let i=0;i<120&&!w.document.getElementById("boot-status").classList.contains("hidden");i++)
    await new Promise(resolve=>setTimeout(resolve,20));
  assert.equal(instance.errors.length,0,instance.errors.join("\n"));
  const meals=w.document.querySelector(".primary-nav .meals-nav");
  assert.ok(meals.classList.contains("hidden"));
  assert.equal(w.document.getElementById("page-meals").classList.contains("hidden"),true);
  assert.equal(w.document.getElementById("page-dashboard").classList.contains("hidden"),false);
  assert.equal(w.document.getElementById("settings-meals-enabled").checked,false);
 }finally{instance.dom.window.close()}
});
test("administrator can disable Meal Planning without losing any other feature setting",async()=>{
 const instance=fixture("https://calden.example/settings/optional-features",{mealsEnabled:true});
 try{
  const {w}=instance;
  for(let i=0;i<120&&!w.document.getElementById("boot-status").classList.contains("hidden");i++)
    await new Promise(resolve=>setTimeout(resolve,20));
  assert.equal(instance.errors.length,0,instance.errors.join("\n"));
  const toggle=w.document.getElementById("settings-meals-enabled");
  assert.equal(toggle.checked,true);
  toggle.checked=false;toggle.dispatchEvent(new w.Event("change",{bubbles:true}));
  for(let i=0;i<120&&toggle.disabled;i++)
    await new Promise(resolve=>setTimeout(resolve,20));
  assert.deepEqual(instance.featureSaves,[{meals_enabled:false}]);
  assert.equal(instance.householdSettings().meals_enabled,false);
  assert.equal(w.document.querySelector(".primary-nav .meals-nav").classList.contains("hidden"),true);
  assert.match(w.document.getElementById("settings-feature-status").textContent,/Meal Planning disabled/);
 }finally{instance.dom.window.close()}
});
