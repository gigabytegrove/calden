window.addEventListener("error",()=>showBootFailure());
window.addEventListener("unhandledrejection",()=>showBootFailure());

function showBootFailure(){
  const el=document.querySelector("#boot-status");
  if(!el)return;
  el.classList.remove("hidden");
  el.classList.add("boot-error");
  el.textContent="CalDen could not finish loading. Refresh the page. If this continues, check the CalDen container logs.";
}

function hideBootStatus(){
  const el=document.querySelector("#boot-status");
  if(el)el.classList.add("hidden");
}

const $=s=>document.querySelector(s), $$=s=>document.querySelectorAll(s);
const state={token:localStorage.getItem("calden_token")||"",me:null,settings:null,users:[],calendars:[],events:[],editingEvent:null,setupStep:0};

async function api(path,options={}){
  const headers={"Content-Type":"application/json",...(options.headers||{})};
  if(state.token) headers.Authorization=`Bearer ${state.token}`;
  const res=await fetch(path,{...options,headers});
  const body=res.status===204?null:await res.json().catch(()=>null);
  if(!res.ok) throw new Error(body?.error||"Something went wrong");
  return body;
}
function setToken(token){state.token=token||"";if(token)localStorage.setItem("calden_token",token);else localStorage.removeItem("calden_token")}
function formJSON(form){return Object.fromEntries(new FormData(form).entries())}
function showAuth(which){
  hideBootStatus();
  $("#app").classList.add("hidden");
  $("#auth").classList.remove("hidden");
  $("#setup-form").classList.toggle("hidden",which!=="setup");
  $("#login-form").classList.toggle("hidden",which!=="login");
  $("#auth-card").classList.toggle("setup-mode",which==="setup");
  $("#auth-error").textContent="";
  if(which==="setup"){
    const zone=Intl.DateTimeFormat().resolvedOptions().timeZone||"UTC";
    const form=$("#setup-form");
    form.timezone.value=zone;
    $("#setup-timezone").textContent=zone.replaceAll("_"," ");
    setSetupStep(0);
  }
}

async function boot(){
  const setup=await api("/api/setup/status");
  if(setup.needs_setup){showAuth("setup");return}
  if(!state.token){showAuth("login");return}
  try{
    state.me=await api("/api/me");
    state.settings=await api("/api/settings/general");
    await reloadSharedData();
    render();
  }catch(e){setToken("");showAuth("login")}
}
async function reloadSharedData(){
  [state.users,state.calendars]=await Promise.all([api("/api/users"),api("/api/calendars")]);
  await loadEvents();
}
async function loadEvents(){
  const from=new Date();from.setDate(from.getDate()-1);
  const to=new Date();to.setDate(to.getDate()+60);
  state.events=await api(`/api/events?from=${encodeURIComponent(from.toISOString())}&to=${encodeURIComponent(to.toISOString())}`);
}
function render(){
  hideBootStatus();
  $("#auth").classList.add("hidden");$("#app").classList.remove("hidden");
  $("#me").textContent=state.me.display_name;
  $("#household-label").textContent=state.settings?.household_name||"";
  $("#manage").classList.toggle("hidden",state.me.role!=="admin");
  $("#today-label").textContent=new Intl.DateTimeFormat(undefined,{weekday:"long",month:"long",day:"numeric"}).format(new Date());
  $("#calendar-strip").innerHTML=state.calendars.map(c=>`<button class="calendar-pill"><span class="dot" style="--cal:${safeColor(c.color)}"></span>${escapeHTML(c.name)}</button>`).join("");
  $("#calendar-select").innerHTML='<option value="">Choose a calendar</option>'+state.calendars.filter(c=>c.can_edit).map(c=>`<option value="${c.id}">${escapeHTML(c.name)}</option>`).join("");
  $("#people-picker").innerHTML=state.users.map(personChoice).join("");
  $("#event-count").textContent=`${state.events.length} event${state.events.length===1?"":"s"}`;
  $("#events").innerHTML=state.events.length?state.events.map(eventCard).join(""):'<div class="empty">Nothing coming up yet. Add an event when you are ready.</div>';
  renderManage();
}
function renderManage(){
  $("#people-list").innerHTML=state.users.map(u=>`<div class="list-row"><span class="avatar">${escapeHTML(u.initials)}</span><div><strong>${escapeHTML(u.display_name)}</strong><small>${escapeHTML(u.username)} · ${u.role==="admin"?"Administrator":u.role==="restricted"?"Restricted member":"Family member"}</small></div></div>`).join("");
  $("#calendar-list").innerHTML=state.calendars.map(c=>`<div class="list-row"><span class="swatch" style="--cal:${safeColor(c.color)}"></span><div><strong>${escapeHTML(c.name)}</strong><small>${escapeHTML(c.description||"")}</small></div></div>`).join("")||'<p class="muted">No calendars yet.</p>';
  const checks=state.users.map(u=>`<label class="plain-check"><input type="checkbox" value="${u.id}"><span>${escapeHTML(u.display_name)}</span></label>`).join("");
  $("#calendar-viewers").innerHTML=checks;$("#calendar-editors").innerHTML=checks;
}
function personChoice(u){return `<label class="person-check"><input type="checkbox" name="assignee" value="${u.id}"><span><i class="avatar">${escapeHTML(u.initials)}</i>${escapeHTML(u.display_name)}</span></label>`}
function eventCard(e){
  const start=new Date(e.starts_at);
  const when=e.all_day?"All day":new Intl.DateTimeFormat(undefined,{hour:"numeric",minute:"2-digit"}).format(start);
  const day=new Intl.DateTimeFormat(undefined,{month:"short",day:"numeric"}).format(start);
  const avatars=(e.assignees||[]).map(u=>`<span class="avatar" title="${escapeHTML(u.display_name)}">${u.avatar_url?`<img src="${escapeAttr(u.avatar_url)}" alt="">`:escapeHTML(u.initials)}</span>`).join("");
  return `<article class="event" style="--cal:${safeColor(e.color)}"><div class="event-time">${escapeHTML(day)}<div class="event-meta">${escapeHTML(when)}</div></div><div><div class="event-title">${escapeHTML(e.title)}</div><div class="event-meta">${escapeHTML(e.calendar_name)}${e.location?" · "+escapeHTML(e.location):""}</div></div><div class="event-actions"><div class="avatars">${avatars}</div><button class="quiet event-edit" data-event-id="${e.id}" type="button">Edit</button></div></article>`;
}
function openEvent(existing=null){
  if(!state.calendars.some(c=>c.can_edit)){if(state.me.role==="admin")openManage();else alert("You do not have a calendar you can add events to yet.");return}
  const f=$("#event-form"); f.reset(); state.editingEvent=existing;
  $("#event-dialog-title").textContent=existing?"Edit event":"Add event";
  $("#delete-event").classList.toggle("hidden",!existing);
  if(existing){
    f.event_id.value=existing.id;
    f.title.value=existing.title;
    f.calendar_id.value=existing.calendar_id;
    f.starts_at.value=localInput(new Date(existing.starts_at));
    f.ends_at.value=localInput(new Date(existing.ends_at));
    f.location.value=existing.location||"";
    f.notes.value=existing.notes||"";
    const personal=(existing.reminders||[]).find(r=>r.kind==="personal"&&r.provider==="android");
    const system=(existing.reminders||[]).find(r=>r.kind==="system"&&r.provider==="monita");
    f.personal_reminder.value=personal?String(personal.minutes_before):"";
    f.system_reminder_enabled.checked=!!system;
    f.system_reminder.value=system?String(system.minutes_before):"1440";
    [...f.querySelectorAll('input[name="assignee"]')].forEach(i=>i.checked=(existing.assignees||[]).some(a=>a.id===i.value));
  }else{
    const start=new Date(Date.now()+3600000);start.setMinutes(0,0,0);const end=new Date(start.getTime()+3600000);
    f.starts_at.value=localInput(start);f.ends_at.value=localInput(end);f.system_reminder.value="1440";
  }
  $("#system-reminder-time").classList.toggle("hidden",!f.system_reminder_enabled.checked);
  $("#event-error").textContent="";$("#event-dialog").showModal();
}
async function openManage(){
  renderManage();$("#manage-dialog").showModal();
  if(state.me.role==="admin"){
    try{
      const m=await api("/api/integrations/monita"),f=$("#monita-form");
      f.enabled.checked=!!m.enabled;f.server_url.value=m.server_url||"";f.token.value=m.token||"";f.default_channel.value=m.default_channel||"";
    }catch{}
  }
}
function setSetupStep(step){
  state.setupStep=Math.max(0,Math.min(3,step));
  $$(".setup-step").forEach(s=>s.classList.toggle("hidden",Number(s.dataset.step)!==state.setupStep));
  $$(".setup-dot").forEach(d=>d.classList.toggle("active",Number(d.dataset.dot)<=state.setupStep));
  $("#setup-back").classList.toggle("hidden",state.setupStep===0);
  $("#setup-next").classList.toggle("hidden",state.setupStep===3);
  $("#setup-finish").classList.toggle("hidden",state.setupStep!==3);
  if(state.setupStep===3) updateSetupSummary();
}

function setupStepValid(){
  const form=$("#setup-form");
  $("#auth-error").textContent="";
  if(state.setupStep===0){
    if(!form.household_name.value.trim()){
      $("#auth-error").textContent="Give your family calendar a name.";
      form.household_name.focus();
      return false;
    }
  }
  if(state.setupStep===1){
    if(!form.display_name.value.trim()){
      $("#auth-error").textContent="Enter your name.";
      form.display_name.focus();
      return false;
    }
    if(form.username.value.trim().length<3){
      $("#auth-error").textContent="Username must be at least 3 characters.";
      form.username.focus();
      return false;
    }
    if(form.password.value.length<8){
      $("#auth-error").textContent="Password must be at least 8 characters.";
      form.password.focus();
      return false;
    }
    if(form.password.value!==form.confirm_password.value){
      $("#auth-error").textContent="Those passwords do not match.";
      form.confirm_password.focus();
      return false;
    }
  }
  return true;
}

function updateSetupSummary(){
  const form=$("#setup-form");
  const names=[...form.querySelectorAll('input[name="starter_calendar"]:checked')].map(i=>i.closest("label").querySelector("b").textContent);
  $("#summary-household").textContent=form.household_name.value.trim();
  $("#summary-admin").textContent=form.display_name.value.trim();
  $("#summary-timezone").textContent=(form.timezone.value||"UTC").replaceAll("_"," ");
  $("#summary-calendars").textContent=names.length?names.join(", "):"Family";
}

function localInput(d){const p=n=>String(n).padStart(2,"0");return `${d.getFullYear()}-${p(d.getMonth()+1)}-${p(d.getDate())}T${p(d.getHours())}:${p(d.getMinutes())}`}
function escapeHTML(v=""){return String(v).replace(/[&<>"']/g,c=>({"&":"&amp;","<":"&lt;",">":"&gt;",'"':"&quot;","'":"&#39;"}[c]))}
function escapeAttr(v=""){return escapeHTML(v)}
function safeColor(v){return /^#[0-9a-f]{6}$/i.test(v)?v:"#667085"}

$("#setup-next").addEventListener("click",()=>{if(setupStepValid())setSetupStep(state.setupStep+1)});
$("#setup-back").addEventListener("click",()=>setSetupStep(state.setupStep-1));
$("#setup-form").addEventListener("submit",async e=>{
  e.preventDefault();
  if(!setupStepValid())return;
  const f=e.currentTarget;
  $("#auth-error").textContent="";
  $("#setup-finish").disabled=true;
  $("#setup-finish").textContent="Setting up…";
  const payload={
    household_name:f.household_name.value.trim(),
    timezone:f.timezone.value||"UTC",
    display_name:f.display_name.value.trim(),
    username:f.username.value.trim(),
    password:f.password.value,
    starter_calendars:[...f.querySelectorAll('input[name="starter_calendar"]:checked')].map(i=>i.value)
  };
  try{
    const out=await api("/api/setup",{method:"POST",body:JSON.stringify(payload)});
    setToken(out.token);
    await boot();
  }catch(err){
    $("#auth-error").textContent=err.message;
  }finally{
    $("#setup-finish").disabled=false;
    $("#setup-finish").textContent="Finish setup";
  }
});
$("#login-form").addEventListener("submit",async e=>{e.preventDefault();$("#auth-error").textContent="";try{const out=await api("/api/login",{method:"POST",body:JSON.stringify(formJSON(e.currentTarget))});setToken(out.token);await boot()}catch(err){$("#auth-error").textContent=err.message}});
$("#logout").addEventListener("click",()=>{setToken("");showAuth("login")});
$("#new-event").addEventListener("click",()=>openEvent());$("#nav-add").addEventListener("click",()=>openEvent());
$("#manage").addEventListener("click",openManage);$("#nav-more").addEventListener("click",()=>{if(state.me.role==="admin")openManage()});
$$("[data-close-event]").forEach(b=>b.addEventListener("click",()=>$("#event-dialog").close()));
$$("[data-close-manage]").forEach(b=>b.addEventListener("click",()=>$("#manage-dialog").close()));
$("#events").addEventListener("click",e=>{const b=e.target.closest(".event-edit");if(!b)return;const ev=state.events.find(x=>x.id===b.dataset.eventId);if(ev)openEvent(ev)});
$("#event-form").system_reminder_enabled.addEventListener("change",e=>$("#system-reminder-time").classList.toggle("hidden",!e.target.checked));
$("#delete-event").addEventListener("click",async()=>{
  if(!state.editingEvent||!confirm("Delete this event?"))return;
  try{await api("/api/events/"+state.editingEvent.id,{method:"DELETE"});$("#event-dialog").close();state.editingEvent=null;await loadEvents();render()}catch(err){$("#event-error").textContent=err.message}
});

$("#person-form").addEventListener("submit",async e=>{
  e.preventDefault();$("#person-error").textContent="";
  try{await api("/api/users",{method:"POST",body:JSON.stringify(formJSON(e.currentTarget))});e.currentTarget.reset();state.users=await api("/api/users");render()}catch(err){$("#person-error").textContent=err.message}
});
$("#calendar-form").addEventListener("submit",async e=>{
  e.preventDefault();$("#calendar-error").textContent="";
  const f=e.currentTarget,fd=new FormData(f);
  const visible=[...$("#calendar-viewers").querySelectorAll("input:checked")].map(i=>i.value);
  const editors=[...$("#calendar-editors").querySelectorAll("input:checked")].map(i=>i.value);
  if(!visible.includes(state.me.id))visible.push(state.me.id);
  if(!editors.includes(state.me.id))editors.push(state.me.id);
  const payload={name:fd.get("name"),color:fd.get("color"),icon:"calendar",description:fd.get("description"),visible_to:visible,editable_by:editors};
  try{await api("/api/calendars",{method:"POST",body:JSON.stringify(payload)});f.reset();f.color.value="#2f6fed";state.calendars=await api("/api/calendars");render()}catch(err){$("#calendar-error").textContent=err.message}
});
$("#event-form").addEventListener("submit",async e=>{
  e.preventDefault();$("#event-error").textContent="";
  const f=e.currentTarget,fd=new FormData(f),reminder=fd.get("personal_reminder"),systemEnabled=f.system_reminder_enabled.checked;
  const reminders=[];
  if(reminder)reminders.push({kind:"personal",provider:"android",minutes_before:Number(reminder),destination:""});
  if(systemEnabled)reminders.push({kind:"system",provider:"monita",minutes_before:Number(fd.get("system_reminder")||1440),destination:""});
  const payload={title:fd.get("title"),calendar_id:fd.get("calendar_id"),starts_at:new Date(fd.get("starts_at")).toISOString(),ends_at:new Date(fd.get("ends_at")).toISOString(),all_day:false,location:fd.get("location"),notes:fd.get("notes"),assignee_ids:fd.getAll("assignee"),reminders};
  const target=state.editingEvent?"/api/events/"+state.editingEvent.id:"/api/events";
  const method=state.editingEvent?"PUT":"POST";
  try{await api(target,{method,body:JSON.stringify(payload)});$("#event-dialog").close();state.editingEvent=null;await loadEvents();render()}catch(err){$("#event-error").textContent=err.message}
});
$("#monita-form").addEventListener("submit",async e=>{
  e.preventDefault();const f=e.currentTarget;$("#monita-status").textContent="";
  const payload={enabled:f.enabled.checked,server_url:f.server_url.value,token:f.token.value,default_channel:f.default_channel.value};
  try{await api("/api/integrations/monita",{method:"PUT",body:JSON.stringify(payload)});$("#monita-status").textContent="Monita settings saved."}catch(err){$("#monita-status").textContent=err.message}
});
$("#test-monita").addEventListener("click",async()=>{
  $("#monita-status").textContent="Sending test…";
  try{await api("/api/integrations/monita/test",{method:"POST",body:"{}"});$("#monita-status").textContent="Test reminder sent."}catch(err){$("#monita-status").textContent=err.message}
});
boot().catch(()=>showAuth("login"));
