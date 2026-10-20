;(function caldenChildrenClient(){
 const form=$("#children-form"),field=n=>form.elements.namedItem(n);
 const status=t=>{$("#children-status").textContent=t||""};
 const moduleLabels={calendar:"Calendars & events",chores:"Assigned chores",tasks:"Assigned tasks",routines:"Daily routines",rewards:"Points & rewards",shopping:"View shared shopping lists",notifications:"Notifications",dashboard:"My home"};
 const defaults=["calendar","chores","routines","rewards"];
 let profiles=[];
 const manager=()=>state.me?.role==="admin";
 const isAdult=u=>u.active!==false&&!u.is_child&&u.role!=="restricted";
 function checkboxList(host,items,selected,prefix){
   host.innerHTML=items.map(item=>`<label class="family-child-choice">
     <input type="checkbox" name="${prefix}" value="${escapeAttr(item.id)}" ${selected.includes(item.id)?"checked":""}>
     <span>${escapeHTML(item.label)}</span></label>`).join("");
 }
 function getChecks(name){return [...form.querySelectorAll('[name="'+name+'"]:checked')].map(x=>x.value)}
 function renderOptions(profile=null){
   const guardians=state.users.filter(isAdult).map(u=>({id:u.id,label:u.display_name}));
   checkboxList($("#children-guardians"),guardians,profile?.guardian_ids||[state.me.id],"guardian");
   checkboxList($("#children-modules"),Object.entries(moduleLabels).map(([id,label])=>({id,label})),
     profile?.allowed_modules||defaults,"child_module");
 }
 function reset(){
   form.reset();field("id").value="";field("display_name").disabled=false;
   field("username").disabled=false;field("username").required=true;
   field("password").required=true;field("password").value="";
   $("#child-password-row").classList.remove("hidden");
   $("#children-form-title").textContent="Add a child account";
   field("display_mode").value="simple";
   renderOptions();
 }
 function edit(profile){
   if(!profile||!manager())return;
   form.reset();field("id").value=profile.id;
   field("display_name").value=profile.display_name;
   field("display_name").disabled=true;
   field("username").value=profile.username;
   field("username").disabled=true;field("username").required=false;
   field("password").value="";field("password").required=false;
   $("#child-password-row").classList.add("hidden");
   field("display_mode").value=profile.display_mode;
   $("#children-form-title").textContent="Edit "+profile.display_name+"'s access";
   renderOptions(profile);
   form.scrollIntoView({block:"nearest",behavior:"smooth"});
 }
 function render(){
   if(!manager())return;
   $("#children-list").innerHTML=profiles.length?profiles.map(p=>
     `<div class="family-child-entry"><div><strong>${escapeHTML(p.display_name)}</strong>
       <small>@${escapeHTML(p.username)} · ${p.active?"Active":"Inactive"}</small>
       <span class="muted">${p.guardian_ids.length} guardian${p.guardian_ids.length===1?"":"s"} · ${p.allowed_modules.length} features</span></div>
       <button type="button" class="button secondary compact" data-child-edit="${escapeAttr(p.id)}">Manage</button></div>`).join("")
     :'<p class="muted">No child accounts configured yet.</p>';
   if(!field("id").value)renderOptions();
 }
 async function load(){
   if(!manager())return;
   try{
     const response=await api("/api/family/children");
     profiles=response.items||[];
     render();status("");
   }catch(err){status("Couldn't load child accounts: "+err.message)}
 }
 $("#children-refresh").addEventListener("click",load);
 $("#children-cancel").addEventListener("click",reset);
 $("#children-list").addEventListener("click",event=>{
   const b=event.target.closest("[data-child-edit]");
   if(b)edit(profiles.find(p=>p.id===b.dataset.childEdit));
 });
 form.addEventListener("submit",async event=>{
   event.preventDefault();if(!manager())return;
   const guardians=getChecks("guardian"),allowed_modules=getChecks("child_module");
   if(!guardians.length){status("Choose at least one adult guardian.");return}
   if(!allowed_modules.length){status("Enable at least one feature.");return}
   const payload={guardian_ids:guardians,allowed_modules,display_mode:field("display_mode").value};
   const id=field("id").value;
   if(!id){
     payload.username=field("username").value.trim();
     payload.display_name=field("display_name").value.trim();
     payload.password=field("password").value;
   }
   const button=$("#children-save");button.disabled=true;
   try{
     await api(id?"/api/family/children/"+encodeURIComponent(id):"/api/family/children",
       {method:id?"PUT":"POST",body:JSON.stringify(payload)});
     state.users=await api("/api/users");
     reset();await load();renderPeople();renderCalendarPermissionChecks();
     status(id?"Child permissions saved.":"Child account created with guardian restrictions.");
   }catch(err){status("Couldn't save child account: "+err.message)}
   finally{button.disabled=false}
 });
 // Apply child-appropriate navigation without trusting frontend switches as authorization.
 function applyChildView(){
   const a=state.me;if(!a?.is_child)return;
   const modules=new Set(a.child_modules||[]);
   document.body.classList.add("calden-child-mode");
   $("#sidebar-role").textContent="Family child account";
   $$(".primary-nav [data-page]").forEach(link=>{
     const page=link.dataset.page;
     const permitted={
       dashboard:true,calendar:modules.has("calendar"),
       agenda:modules.has("calendar")&&state.settings?.agenda_enabled!==false,
       chores:modules.has("chores")||modules.has("tasks"),
       routines:modules.has("routines"),rewards:modules.has("rewards"),
       shopping:modules.has("shopping"),notifications:true
     };
     if(!permitted[page])link.classList.add("hidden");
   });
   $("#dashboard-layout").value=a.child_layout||"simple";
   $("#dashboard-layout").disabled=true;
   // A child must not be able to browse bill details or edit household roles.
   $$(".admin-only,.calendar-manager,.admin-settings").forEach(x=>x.classList.add("hidden"));
   $("#new-event").classList.add("hidden");
 }
 window.caldenChildren={load,render,applyChildView,enter(page){if(page==="people")load()}};
})();
