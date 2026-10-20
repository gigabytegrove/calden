// Calden household modules, personal dashboard, and theme controls.
// This extends the established app shell and uses the existing authenticated API.
const familyModuleInfo={
 shopping:{title:"Shopping lists",intro:"Make shared shopping lists and mark what you have picked up.",subject:"Item for",group:"List",quantity:true,due:"Needed by"},
 school:{title:"School activities",intro:"Keep school activities, assignments and important dates together.",subject:"Student or school",group:"Class or group",due:"Due date"},
 maintenance:{title:"Home maintenance",intro:"Track home jobs, equipment, and repeating maintenance.",subject:"Equipment or area",group:"Location",due:"Next due"},
 pets:{title:"Pet care",intro:"Keep track of appointments, medicines and regular pet care.",subject:"Pet name",group:"Care category",due:"Due date"},
 routines:{title:"Family routines",intro:"Set up repeating morning, evening and custom checklists.",subject:"Person or group",group:"Time of day",due:"First day"}
};
const familyModules=Object.keys(familyModuleInfo);
const dashboardWidgetNames={calendar:"My calendar",notifications:"My reminders",tasks:"Tasks",chores:"Chores",bills:"Bills",agenda:"Upcoming plans",
 shopping:"Shopping",school:"School",maintenance:"Home maintenance",pets:"Pet care",routines:"Routines"};
const familyIconNames={"shopping-cart":"🛒","school":"🎓","house":"🏠","paw-print":"🐾","list-checks":"☑","calendar":"📅","star":"⭐","heart":"♥","clipboard-check":"📋"};
const familyDefaultIcons={shopping:"shopping-cart",school:"school",maintenance:"house",pets:"paw-print",routines:"list-checks"};
const familyPageCache={};
function familyToday(){return dateInputValue(new Date())}
function familyName(section){
 return state.familyAppearance?.labels?.[section]||familyModuleInfo[section]?.title||section;
}
function applyFamilyTheme(){
 const appearance=state.dashboardPreferences?.appearance||"system";
 const dark=appearance==="dark"||(appearance==="system"&&window.matchMedia?.("(prefers-color-scheme: dark)")?.matches);
 document.body.classList.toggle("calden-dark",!!dark);
 const preset=state.familyAppearance?.preset||"ocean";
 document.body.dataset.familyTheme=preset;
 document.body.classList.toggle("calden-simple",state.dashboardPreferences?.layout==="simple");
}
function updateFamilyNavigation(){
 document.querySelectorAll(".family-nav").forEach(link=>{
  const section=link.dataset.page;
  link.classList.toggle("hidden",!state.familyFeatures?.[section]);
  const label=link.querySelector(".nav-label");
  if(label)label.textContent=familyName(section);
  let icon=link.querySelector(".family-nav-icon");
  if(!icon){icon=document.createElement("span");icon.className="family-nav-icon";icon.setAttribute("aria-hidden","true");link.insertBefore(icon,label)}
  icon.textContent=familyIconNames[state.familyAppearance?.icons?.[section]||familyDefaultIcons[section]]||"•";
 });
 const homeLink=document.querySelector('[data-page="home"]');
 if(homeLink)homeLink.classList.remove("hidden");
 applyFamilyTheme();
}
function renderDashboardHome(){
 const host=document.querySelector("#family-dashboard-widgets");
 if(!host||!state.me)return;
 const prefs=state.dashboardPreferences||{widgets:["calendar","notifications"]};
 const widgets=Array.isArray(prefs.widgets)?prefs.widgets:[];
 const me=state.me.id;
 const events=(state.events||[]).filter(e=>Array.isArray(e.assignees)&&e.assignees.some(p=>p.id===me));
 const cards={
  calendar:{rows:events.slice(0,5).map(e=>e.title+" · "+formatDate(eventStartDate(e)))},
  agenda:{rows:(state.events||[]).slice(0,5).map(e=>e.title+" · "+formatDate(eventStartDate(e)))},
  bills:{rows:(state.events||[]).filter(e=>isBillCalendar(e.calendar_id)).slice(0,5).map(e=>e.title+" · "+billPaymentStatus(e))},
  notifications:{rows:(state.notifications||[]).slice(0,5).map(n=>n.title||n.message||"Reminder")},
  tasks:{rows:(state.chores||[]).filter(c=>c.kind==="task").slice(0,5).map(c=>c.title)},
  chores:{rows:(state.chores||[]).filter(c=>c.kind!=="task").slice(0,5).map(c=>c.title)}
 };
 const visible=widgets.filter(k=>dashboardWidgetNames[k]&&(cards[k]||state.familyFeatures?.[k]));
 host.innerHTML=visible.map(k=>{
  const rows=(cards[k]?.rows||familyPageCache[k]?.items?.slice(0,5).map(i=>i.title)||[]);
  return '<article class="panel family-dashboard-card"><div class="family-card-head"><h3>'+escapeHTML(dashboardWidgetNames[k])+'</h3><a href="/'+(k==="calendar"?"calendar":k==="notifications"?"notifications":k==="tasks"||k==="chores"?"chores":k==="bills"?"bills":k==="agenda"?"agenda":k)+'" data-family-quick="'+k+'">Open</a></div>'+
   (rows.length?'<ul>'+rows.map(x=>'<li>'+escapeHTML(x)+'</li>').join("")+'</ul>':'<p class="muted">Nothing to show yet.</p>')+'</article>';
 }).join("")||'<section class="panel stack"><h3>Make this space yours</h3><p class="muted">Choose your widgets under Settings → My dashboard.</p></section>';
 host.querySelectorAll("[data-family-quick]").forEach(a=>a.addEventListener("click",e=>{
  e.preventDefault();navigate(a.dataset.familyQuick==="tasks"?"chores":a.dataset.familyQuick);
 }));
}
async function refreshFamilyDashboard(){
 renderDashboardHome();
 if(state.settings?.tasks_enabled||state.settings?.chores_enabled){
  try{await loadChores()}catch{}
  renderDashboardHome();
 }
}
async function loadFamilySection(section){
 if(!state.familyFeatures?.[section])return;
 const host=document.querySelector("#family-list-"+section),date=document.querySelector("#family-date-"+section);
 if(!host)return;
 host.innerHTML='<p class="muted">Loading '+escapeHTML(familyName(section))+'…</p>';
 try{
  const data=await api("/api/household/"+section+"?on="+encodeURIComponent(date.value||familyToday()));
  familyPageCache[section]=data;
  renderFamilyItems(section);
 }catch(err){host.textContent="Could not load this section: "+err.message}
}
function familyOccurrenceValid(item,on){
 if(item.recurrence==="none")return true;
 if(!item.due_on||on<item.due_on)return false;
 const a=calendarDate(item.due_on),b=calendarDate(on);
 const days=Math.round((Date.UTC(b.getFullYear(),b.getMonth(),b.getDate())-Date.UTC(a.getFullYear(),a.getMonth(),a.getDate()))/86400000);
 if(item.recurrence==="daily")return true;
 if(item.recurrence==="weekly")return days%7===0;
 if(item.recurrence==="biweekly")return days%14===0;
 if(item.recurrence==="monthly")return b.getDate()===Math.min(a.getDate(),new Date(b.getFullYear(),b.getMonth()+1,0).getDate());
 if(item.recurrence==="yearly")return b.getMonth()===a.getMonth()&&b.getDate()===Math.min(a.getDate(),new Date(b.getFullYear(),b.getMonth()+1,0).getDate());
 return false;
}
function renderFamilyItems(section){
 const host=document.querySelector("#family-list-"+section);
 const data=familyPageCache[section]||{items:[],on:familyToday()};
 const on=data.on||familyToday();
 const items=data.items||[];
 const heading=document.querySelector("#family-count-"+section);
 if(heading)heading.textContent=items.length+" saved "+(items.length===1?"item":"items");
 host.innerHTML=items.length?items.map(i=>{
  const canDo=familyOccurrenceValid(i,on);
  const summary=[i.subject,i.list_name,i.due_on?"Due "+i.due_on:"",i.recurrence!=="none"?"Repeats "+i.recurrence:"",section==="shopping"?i.quantity+" "+i.unit:""].filter(Boolean).join(" · ");
  return '<article class="panel family-entry'+(i.done?" family-entry-done":"")+'"><div class="family-entry-main"><strong>'+escapeHTML(i.title)+'</strong>'+
   (i.private?'<span class="family-private">Private</span>':"")+
   '<p class="muted">'+escapeHTML(summary)+'</p>'+
   (i.notes?'<p>'+escapeHTML(i.notes)+'</p>':"")+'</div>'+
   '<div class="family-entry-actions"><button type="button" class="button secondary compact" data-family-done="'+escapeAttr(i.id)+'" '+(canDo?"":"disabled title=\"Not scheduled for the selected date\"")+'>'+
   (i.done?"Undo":"Mark done")+'</button>'+
   (i.can_edit?'<button class="button secondary compact" type="button" data-family-edit="'+escapeAttr(i.id)+'">Edit</button><button class="button secondary compact" type="button" data-family-remove="'+escapeAttr(i.id)+'">Delete</button>':"")+
   '</div></article>';
 }).join(""):'<div class="panel family-empty"><h3>Nothing here yet</h3><p class="muted">Add your first item to get organized.</p></div>';
}
function familyResetForm(section,item=null){
 const form=document.querySelector("#family-form-"+section);
 if(!form)return;
 form.reset();
 form.elements.namedItem("id").value=item?.id||"";
 for(const key of ["title","notes","subject","list_name","quantity","unit","due_on","recurrence"]){
  const el=form.elements.namedItem(key);
  if(el)el.value=item?.[key]??(key==="quantity"?"1":key==="recurrence"?"none":"");
 }
 form.elements.namedItem("private").checked=!!item?.private;
 form.querySelector(".family-form-title").textContent=item?"Edit "+familyName(section):"Add to "+familyName(section);
 form.classList.remove("hidden");
 form.elements.namedItem("title").focus();
}
function familyEditorFields(section){
 const root=document.querySelector("#page-"+section),info=familyModuleInfo[section];
 if(!root)return;
 root.querySelector(".family-page-title").textContent=familyName(section);
 root.querySelector(".family-page-intro").textContent=info.intro;
 root.querySelector("[data-family-subject-label]").textContent=info.subject;
 root.querySelector("[data-family-group-label]").textContent=info.group;
 root.querySelector("[data-family-due-label]").textContent=info.due;
 root.querySelector(".family-quantity-fields").classList.toggle("hidden",!info.quantity);
 const date=root.querySelector(".family-on-date");
 if(!date.value)date.value=familyToday();
}
function renderFamilySettings(){
 const host=document.querySelector("#family-feature-controls");
 if(host&&state.me?.role==="admin"){
  host.innerHTML=familyModules.map(section=>'<label class="toggle-row"><input type="checkbox" data-family-feature="'+section+'" '+(state.familyFeatures?.[section]?"checked":"")+'><span>Enable '+escapeHTML(familyName(section))+'</span></label>').join("");
 }
 const preset=document.querySelector("#family-preset");
 if(preset)preset.value=state.familyAppearance?.preset||"ocean";
 const labels=document.querySelector("#family-custom-labels");
 if(labels)labels.innerHTML=familyModules.map(section=>'<div class="family-label-row"><label>'+escapeHTML(familyModuleInfo[section].title)+
  '<input type="text" maxlength="40" data-family-label="'+section+'" value="'+escapeAttr(state.familyAppearance?.labels?.[section]||"")+'" placeholder="'+escapeAttr(familyModuleInfo[section].title)+'"></label>'+
  '<label>Icon<select data-family-icon="'+section+'">'+Object.keys(familyIconNames).map(icon=>'<option value="'+icon+'" '+((state.familyAppearance?.icons?.[section]||familyDefaultIcons[section])===icon?"selected":"")+'>'+familyIconNames[icon]+' '+escapeHTML(icon.replaceAll("-"," "))+'</option>').join("")+'</select></label></div>').join("");
 const prefs=state.dashboardPreferences||{appearance:"system",layout:"standard",widgets:["calendar","notifications"]};
 const appearance=document.querySelector("#my-dashboard-appearance"),layout=document.querySelector("#my-dashboard-layout");
 if(appearance)appearance.value=prefs.appearance;
 if(layout)layout.value=prefs.layout;
 renderDashboardPicker();
}
function renderDashboardPicker(){
 const host=document.querySelector("#dashboard-widget-picker");if(!host)return;
 const selected=state.dashboardPreferences?.widgets||[];
 const enabled=["calendar","notifications",...(state.settings?.tasks_enabled?["tasks"]:[]),
 ...(state.settings?.chores_enabled?["chores"]:[]),...(state.settings?.bills_enabled!==false?["bills"]:[]),
 ...(state.settings?.agenda_enabled!==false?["agenda"]:[]),...familyModules.filter(k=>state.familyFeatures?.[k])];
 host.innerHTML=enabled.map(k=>{
  const selectedIndex=selected.indexOf(k);
  return '<div class="family-widget-row"><label><input type="checkbox" data-widget-choice="'+k+'" '+(selectedIndex>=0?"checked":"")+'><span>'+escapeHTML(dashboardWidgetNames[k])+'</span></label>'+
   (selectedIndex>=0?'<span class="family-widget-order"><button type="button" data-widget-up="'+k+'" '+(selectedIndex===0?"disabled":"")+' aria-label="Move '+escapeAttr(dashboardWidgetNames[k])+' up">↑</button>'+
   '<button type="button" data-widget-down="'+k+'" '+(selectedIndex===selected.length-1?"disabled":"")+' aria-label="Move '+escapeAttr(dashboardWidgetNames[k])+' down">↓</button></span>':"")+'</div>';
 }).join("");
}
function initFamilyUI(){
 document.querySelector("#dashboard-personalize")?.addEventListener("click",()=>{navigate("settings");activateSettingsTab("personalize",true)});
 document.querySelectorAll(".family-page").forEach(root=>{
  const section=root.dataset.familySection;
  root.querySelector(".family-add").addEventListener("click",()=>familyResetForm(section));
  root.querySelector(".family-cancel").addEventListener("click",()=>root.querySelector("form").classList.add("hidden"));
  root.querySelector(".family-refresh").addEventListener("click",()=>loadFamilySection(section));
  root.querySelector(".family-on-date").addEventListener("change",()=>loadFamilySection(section));
  root.querySelector("form").addEventListener("submit",async e=>{
   e.preventDefault();
   const form=e.currentTarget,id=form.elements.namedItem("id").value,body={};
   for(const name of ["title","notes","subject","list_name","unit","due_on","recurrence"]){
    body[name]=form.elements.namedItem(name).value;
   }
   body.quantity=Number(form.elements.namedItem("quantity").value)||1;
   body.private=form.elements.namedItem("private").checked;
   const status=root.querySelector(".family-status");
   try{
    await api("/api/household/"+section+(id?"/"+encodeURIComponent(id):""),{method:id?"PUT":"POST",body:JSON.stringify(body)});
    form.classList.add("hidden");status.textContent="Saved.";await loadFamilySection(section);
   }catch(err){status.textContent=err.message}
  });
  root.querySelector(".family-list").addEventListener("click",async e=>{
   const button=e.target.closest("button[data-family-done],button[data-family-edit],button[data-family-remove]");
   if(!button)return;
   const data=familyPageCache[section],id=button.dataset.familyDone||button.dataset.familyEdit||button.dataset.familyRemove;
   const item=data?.items?.find(i=>i.id===id);
   if(!item)return;
   if(button.dataset.familyEdit){familyResetForm(section,item);return}
   if(button.dataset.familyRemove&&!confirm("Delete this item and its completion history?"))return;
   button.disabled=true;
   try{
    if(button.dataset.familyRemove){
     await api("/api/household/"+section+"/"+encodeURIComponent(id),{method:"DELETE"});
    }else{
     await api("/api/household/"+section+"/"+encodeURIComponent(id)+"/completion",{
      method:"PUT",body:JSON.stringify({on:data.on,done:!item.done})
     });
    }
    await loadFamilySection(section);
   }catch(err){root.querySelector(".family-status").textContent=err.message;button.disabled=false}
  });
 });
 document.querySelector("#family-feature-controls")?.addEventListener("change",async e=>{
  const el=e.target.closest("[data-family-feature]");if(!el)return;
  el.disabled=true;
  const old=!el.checked;
  try{
   const result=await api("/api/household/features",{method:"PUT",body:JSON.stringify({features:{[el.dataset.familyFeature]:el.checked}})});
   state.familyFeatures=result.features;
   updateFamilyNavigation();renderFamilySettings();
  }catch(err){el.checked=old;document.querySelector("#family-settings-status").textContent=err.message}
  finally{el.disabled=false}
 });
 document.querySelector("#family-appearance-form")?.addEventListener("submit",async e=>{
  e.preventDefault();
  const preset=document.querySelector("#family-preset").value;
  const labels={},icons={};
  document.querySelectorAll("[data-family-label]").forEach(input=>{if(input.value.trim())labels[input.dataset.familyLabel]=input.value.trim()});
  document.querySelectorAll("[data-family-icon]").forEach(select=>{icons[select.dataset.familyIcon]=select.value});
  try{
   state.familyAppearance=await api("/api/household/appearance",{method:"PUT",body:JSON.stringify({preset,labels,icons})});
   document.querySelector("#family-appearance-status").textContent="Appearance saved for your household.";
   updateFamilyNavigation();familyModules.forEach(familyEditorFields);
  }catch(err){document.querySelector("#family-appearance-status").textContent=err.message}
 });
 document.querySelector("#dashboard-widget-picker")?.addEventListener("click",e=>{
  const b=e.target.closest("[data-widget-up],[data-widget-down]");if(!b)return;
  const selected=[...(state.dashboardPreferences?.widgets||[])];
  const k=b.dataset.widgetUp||b.dataset.widgetDown,i=selected.indexOf(k),next=i+(b.dataset.widgetUp?-1:1);
  if(i<0||next<0||next>=selected.length)return;
  [selected[i],selected[next]]=[selected[next],selected[i]];
  state.dashboardPreferences.widgets=selected;renderDashboardPicker();
 });
 document.querySelector("#dashboard-widget-picker")?.addEventListener("change",e=>{
  const check=e.target.closest("[data-widget-choice]");if(!check)return;
  const widgets=[...(state.dashboardPreferences?.widgets||[])];
  if(check.checked&&!widgets.includes(check.dataset.widgetChoice))widgets.push(check.dataset.widgetChoice);
  if(!check.checked){const i=widgets.indexOf(check.dataset.widgetChoice);if(i>=0)widgets.splice(i,1)}
  state.dashboardPreferences.widgets=widgets;renderDashboardPicker();
 });
 document.querySelector("#my-dashboard-form")?.addEventListener("submit",async e=>{
  e.preventDefault();
  const p={appearance:document.querySelector("#my-dashboard-appearance").value,
   layout:document.querySelector("#my-dashboard-layout").value,
   widgets:[...(state.dashboardPreferences?.widgets||[])]};
  try{
   state.dashboardPreferences=await api("/api/me/dashboard",{method:"PUT",body:JSON.stringify(p)});
   applyFamilyTheme();renderDashboardHome();
   document.querySelector("#my-dashboard-status").textContent="Your dashboard preferences are saved across devices.";
  }catch(err){document.querySelector("#my-dashboard-status").textContent=err.message}
 });
 if(window.matchMedia){
  window.matchMedia("(prefers-color-scheme: dark)").addEventListener?.("change",applyFamilyTheme);
 }
}
initFamilyUI();
