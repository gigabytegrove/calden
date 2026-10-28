"use strict";
const {chromium}=require("playwright");
const assert=require("node:assert/strict");

(async()=>{
 const browser=await chromium.launch({headless:true});
 try{
  for(const width of [360,768,1280]){
   const page=await browser.newPage({viewport:{width,height:900}});
   try{
    await page.goto("http://127.0.0.1:8787/",{waitUntil:"networkidle"});
    await page.locator("#setup-form").waitFor({state:"visible"});
    await page.evaluate(()=>{
     const host=document.createElement("main");
     host.id="ui-contract-fixture";
     host.className="workspace";
     host.style.width="100%";
     host.innerHTML=[
      '<section class="app-page packages-page">',
      '<div class="page-header"><h2>Package tracking</h2></div>',
      '<div class="package-totals">',
      '<div class="package-total"><strong>2</strong><span>Incoming</span></div>',
      '<div class="package-total"><strong>0</strong><span>Expected today</span></div>',
      '<div class="package-total"><strong>1</strong><span>Delivered</span></div></div>',
      '<section class="panel"><label>Find package<input placeholder="Search"></label>',
      '<div class="form-actions"><button class="button" type="button">Add</button>',
      '<button class="button secondary" type="button">Refresh</button></div></section>',
      '<div class="package-list"><article class="package-entry">',
      '<div class="package-entry-main"><div class="package-entry-description">',
      '<strong>Replacement laptop battery</strong>',
      '<span>UPS · 1Z999AA10123456784</span></div></div>',
      '<div class="package-entry-side"><span class="package-status">In transit</span>',
      '<button class="button secondary compact">Tracking history</button></div>',
      '</article></div><section class="family-widget"><h3>Family widget</h3>',
      '<p class="muted">Shared surface</p></section></section>'
     ].join("");
     document.body.append(host);
    });
    const light=await page.evaluate(()=>{
     const el=s=>document.querySelector("#ui-contract-fixture "+s);
     const styles=node=>getComputedStyle(node);
     return {
      panel:styles(el(".panel")).backgroundColor,
      field:styles(el("input")).backgroundColor,
      card:styles(el(".package-entry")).borderRadius,
      inputHeight:el("input").getBoundingClientRect().height,
      buttonHeight:el(".button").getBoundingClientRect().height,
      scrollWidth:el(".app-page").scrollWidth,
      clientWidth:el(".app-page").clientWidth
     };
    });
    assert.equal(light.panel,"rgb(255, 255, 255)",width+" light panel");
    assert.equal(light.field,"rgb(255, 255, 255)",width+" light input");
    assert.ok(light.inputHeight>=40,width+" input too small");
    assert.ok(light.buttonHeight>=39,width+" button too small");
    assert.ok(light.scrollWidth<=light.clientWidth+1,width+" packages page overflow");
    const dark=await page.evaluate(async()=>{
     // Legacy controls animate background-color for 160ms. Verify the
     // settled appearance rather than asserting on the initial white frame.
     document.body.classList.add("calden-family-dark");
     await new Promise(resolve=>setTimeout(resolve,275));
     const el=s=>document.querySelector("#ui-contract-fixture "+s);
     const computed=s=>getComputedStyle(el(s));
     return {
      themeActive:document.body.classList.contains("calden-family-dark"),
      token:getComputedStyle(document.body).getPropertyValue("--ui-surface").trim(),
      panel:computed(".panel").backgroundColor,
      panelToken:computed(".panel").getPropertyValue("--ui-surface").trim(),
      panelInline:el(".panel").getAttribute("style"),
      panelMatches:el(".panel").matches("body.calden-family-dark .workspace .app-page .panel"),
      fieldToken:computed("input").getPropertyValue("--ui-field").trim(),
      fieldMatches:el("input").matches("body.calden-family-dark .workspace .app-page input:not([type=checkbox])"),
      matchedRules:(()=>{
       const output=[];
       const target=el(".panel");
       const walk=rules=>{
        for(const rule of rules){
         if(rule.selectorText&&rule.selectorText.includes(".panel")){
          try{
           if(target.matches(rule.selectorText)&&
            (rule.style?.background||rule.style?.backgroundColor))
             output.push({
              selector:rule.selectorText.slice(0,180),
              bg:rule.style.background||rule.style.backgroundColor,
              priority:rule.style.getPropertyPriority("background")
             });
          }catch{}
         }
         if(rule.cssRules)walk(rule.cssRules);
        }
       };
       for(const sheet of document.styleSheets){
        try{walk(sheet.cssRules)}catch{}
       }
       return output.slice(-16);
      })(),
      field:computed("input").backgroundColor,
      widget:computed(".family-widget").backgroundColor,
      card:computed(".package-entry").backgroundColor,
      overflow:el(".app-page").scrollWidth>el(".app-page").clientWidth+1
     };
    });
    console.log(JSON.stringify({width,light,dark}));
    assert.equal(dark.themeActive,true,width+" dark class not active");
    assert.notEqual(dark.panel,light.panel,width+" dark panel stayed white");
    assert.notEqual(dark.field,light.field,width+" dark field stayed white");
    assert.notEqual(dark.widget,light.panel,width+" family widget stayed white in dark");
    assert.notEqual(dark.card,light.panel,width+" package card stayed white in dark");
    assert.equal(dark.overflow,false,width+" dark packages overflow");
    const history=await page.evaluate(()=>{
     document.body.classList.add("calden-family-dark");
     const dialog=document.getElementById("package-history-dialog");
     dialog.showModal();
     const result={background:getComputedStyle(dialog).backgroundColor};
     dialog.close();
     return result;
    });
    assert.notEqual(history.background,light.panel,width+" history modal stayed white in dark");
    console.log(JSON.stringify({width,light,dark,history}));
   }finally{await page.close()}
  }
 }finally{await browser.close()}
})().catch(error=>{console.error(error);process.exitCode=1});
