;(function caldenAppearance(){
 const defaults={
  dashboard:["My home","house"],calendar:["Calendar","calendar"],agenda:["Agenda","clipboard-list"],
  bills:["Bill Pay","wallet"],tasks:["Tasks","list-check"],chores:["Chores","list-check"],
  shopping:["Shopping","shopping-cart"],routines:["Routines","list-check"],
  rewards:["Rewards & Allowances","gift"],school:["School","graduation-cap"],
  maintenance:["Home upkeep","wrench"],pets:["Pets","paw-print"],notifications:["Notifications","bell"]
 };
 const icons=["grid","calendar","house","shopping-cart","list-check","gift",
   "graduation-cap","wrench","paw-print","bell","wallet","users","utensils","heart","clipboard-list"];
 // All icon geometry comes from this fixed local catalog; never accept SVG or HTML from settings.
 const shapes={
  grid:'<rect x="3" y="3" width="7" height="7" rx="1"/><rect x="14" y="3" width="7" height="7" rx="1"/><rect x="3" y="14" width="7" height="7" rx="1"/><rect x="14" y="14" width="7" height="7" rx="1"/>',
  calendar:'<rect x="3" y="5" width="18" height="16" rx="2"/><path d="M7 3v4m10-4v4M3 10h18"/>',
  house:'<path d="m3 10 9-7 9 7v11h-7v-7h-4v7H3V10Z"/>',
  "shopping-cart":'<path d="M3 4h2l2 11h12l2-8H6m4 13h.01M18 20h.01"/>',
  "list-check":'<path d="m3 7 2 2 4-4m3 2h9M3 17l2 2 4-4m3 2h9"/>',
  gift:'<rect x="3" y="9" width="18" height="12" rx="2"/><path d="M2 9h20M12 9v12M12 9C8 9 5 7 7 4s5 1 5 5Zm0 0c4 0 7-2 5-5s-5 1-5 5Z"/>',
  "graduation-cap":'<path d="m2 9 10-5 10 5-10 5L2 9Zm4 3v5c4 3 8 3 12 0v-5M22 9v8"/>',
  wrench:'<path d="M21 7a7 7 0 0 1-9 8L5 22l-3-3 7-7A7 7 0 0 1 17 3l-4 4 4 4 4-4Z"/>',
  "paw-print":'<path d="M12 13c-3-4-8 1-6 4 2 2 4 2 6 2s4 0 6-2c2-3-3-8-6-4ZM5 9v.01M9 5v.01M15 5v.01M19 9v.01"/>',
  bell:'<path d="M18 9a6 6 0 1 0-12 0c0 7-3 7-3 9h18c0-2-3-2-3-9ZM10 21h4"/>',
  wallet:'<rect x="2" y="5" width="20" height="15" rx="2"/><path d="M3 9h19m-7 6h4"/>',
  users:'<path d="M16 20v-2a4 4 0 0 0-4-4H7a4 4 0 0 0-4 4v2m6-10a4 4 0 1 0 0-8 4 4 0 0 0 0 8Zm9 4a4 4 0 0 1 3 4v2m-4-18a4 4 0 0 1 0 8"/>',
  utensils:'<path d="M4 3v7m4-7v7M6 3v18M3 10h6m9-7v18m0-18c-5 2-5 10 0 11"/>',
  heart:'<path d="M20 8c0 5-8 11-8 11S4 13 4 8a5 5 0 0 1 8-3 5 5 0 0 1 8 3Z"/>',
  "clipboard-list":'<rect x="5" y="4" width="14" height="18" rx="2"/><path d="M9 4V2h6v2M9 10h6m-6 5h6"/>'
 };
 const form=document.getElementById("family-appearance-form"),status=document.getElementById("family-appearance-status");
 const label=(id,fallback)=>state.familyAppearance?.modules?.find(x=>x.module_id===id)?.label||fallback;
 const icon=key=>'<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">'+(shapes[key]||shapes.grid)+'</svg>';
 function apply(){
  const appearance=state.familyAppearance;
  if(!appearance)return;
  document.body.dataset.familyTheme=appearance.theme||"classic";
  for(const module of appearance.modules||[]){
   if(!defaults[module.module_id])continue;
   document.querySelectorAll('.primary-nav .nav-item[data-page="'+module.module_id+'"]').forEach(node=>{
    let glyph=node.querySelector(".nav-icon");
    if(!glyph){glyph=document.createElement("span");glyph.className="nav-icon";node.prepend(glyph)}
    glyph.innerHTML=icon(module.icon);
    let title=node.querySelector(".family-nav-label");
    if(!title){
     title=document.createElement("span");title.className="family-nav-label";
     for(const text of [...node.childNodes]){
      if(text.nodeType===3)text.remove();
     }
     node.appendChild(title);
    }
    title.textContent=module.label;
   });
  }
  // Widget titles can reuse the same stable module IDs; user-specific order remains private.
  window.caldenFamily?.renderDashboard();
 }
 function renderForm(){
  if(!form||!state.familyAppearance||state.me?.role!=="admin")return;
  form.elements.namedItem("theme").value=state.familyAppearance.theme;
  const list=document.getElementById("family-appearance-modules");
  list.innerHTML=(state.familyAppearance.modules||[]).map(m=>
   '<div class="family-appearance-row" data-module="'+escapeAttr(m.module_id)+'">'+
   '<label>'+escapeHTML(defaults[m.module_id]?.[0]||m.module_id)+
   '<input name="label" required maxlength="32" value="'+escapeAttr(m.label)+'"></label>'+
   '<label>Icon<select name="icon">'+icons.map(key=>'<option value="'+escapeAttr(key)+'" '+
    (m.icon===key?"selected":"")+'>'+escapeHTML(key.replaceAll("-"," "))+'</option>').join("")+
   '</select></label></div>').join("");
 }
 async function load(){
  try{
   const response=await api("/api/family/appearance");
   state.familyAppearance=response;
   apply();renderForm();
   return response;
  }catch(err){
   if(state.me?.role==="admin"&&status)status.textContent="Couldn't load family appearance: "+err.message;
   return null;
  }
 }
 form?.addEventListener("submit",async event=>{
  event.preventDefault();
  if(state.me?.role!=="admin")return;
  const modules=[...form.querySelectorAll("[data-module]")].map(row=>({
   module_id:row.dataset.module,
   label:row.querySelector('[name="label"]').value.trim(),
   icon:row.querySelector('[name="icon"]').value
  }));
  const body={theme:form.elements.namedItem("theme").value,modules};
  const button=form.querySelector('[type="submit"]');button.disabled=true;
  try{
   state.familyAppearance=await api("/api/family/appearance",{method:"PUT",body:JSON.stringify(body)});
   apply();renderForm();
   status.textContent="Family appearance saved. Everyone will see these names and theme.";
  }catch(err){status.textContent="Couldn't save appearance: "+err.message}
  finally{button.disabled=false}
 });
 document.getElementById("family-appearance-reset")?.addEventListener("click",()=>{
  if(state.me?.role!=="admin"||!state.familyAppearance)return;
  state.familyAppearance.modules=state.familyAppearance.modules.map(m=>{
   const [name,glyph]=defaults[m.module_id]||[m.label,m.icon];
   return {...m,label:name,icon:glyph};
  });
  renderForm();
  status.textContent="Suggested names restored in the form. Save to apply household-wide.";
 });
 window.caldenAppearance={load,apply,label,renderForm};
})();
