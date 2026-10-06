const $=s=>document.querySelector(s), $$=s=>document.querySelectorAll(s);
const state={token:localStorage.getItem("calden_token")||"",me:null,users:[],calendars:[],events:[]};

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
function showAuth(which){$("#app").classList.add("hidden");$("#auth").classList.remove("hidden");$("#setup-form").classList.toggle("hidden",which!=="setup");$("#login-form").classList.toggle("hidden",which!=="login")}

async function boot(){
  const setup=await api("/api/setup/status");
  if(setup.needs_setup){showAuth("setup");return}
  if(!state.token){showAuth("login");return}
  try{
    state.me=await api("/api/me");
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
  $("#auth").classList.add("hidden");$("#app").classList.remove("hidden");
  $("#me").textContent=state.me.display_name;
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
  return `<article class="event" style="--cal:${safeColor(e.color)}"><div class="event-time">${escapeHTML(day)}<div class="event-meta">${escapeHTML(when)}</div></div><div><div class="event-title">${escapeHTML(e.title)}</div><div class="event-meta">${escapeHTML(e.calendar_name)}${e.location?" · "+escapeHTML(e.location):""}</div></div><div class="avatars">${avatars}</div></article>`;
}
function openEvent(){
  if(!state.calendars.some(c=>c.can_edit)){if(state.me.role==="admin")openManage();else alert("You do not have a calendar you can add events to yet.");return}
  const f=$("#event-form"),start=new Date(Date.now()+3600000);start.setMinutes(0,0,0);const end=new Date(start.getTime()+3600000);
  f.reset();f.starts_at.value=localInput(start);f.ends_at.value=localInput(end);$("#event-error").textContent="";$("#event-dialog").showModal();
}
function openManage(){renderManage();$("#manage-dialog").showModal()}
function localInput(d){const p=n=>String(n).padStart(2,"0");return `${d.getFullYear()}-${p(d.getMonth()+1)}-${p(d.getDate())}T${p(d.getHours())}:${p(d.getMinutes())}`}
function escapeHTML(v=""){return String(v).replace(/[&<>"']/g,c=>({"&":"&amp;","<":"&lt;",">":"&gt;",'"':"&quot;","'":"&#39;"}[c]))}
function escapeAttr(v=""){return escapeHTML(v)}
function safeColor(v){return /^#[0-9a-f]{6}$/i.test(v)?v:"#667085"}

$("#setup-form").addEventListener("submit",async e=>{e.preventDefault();$("#auth-error").textContent="";try{const out=await api("/api/setup",{method:"POST",body:JSON.stringify(formJSON(e.currentTarget))});setToken(out.token);await boot();openManage()}catch(err){$("#auth-error").textContent=err.message}});
$("#login-form").addEventListener("submit",async e=>{e.preventDefault();$("#auth-error").textContent="";try{const out=await api("/api/login",{method:"POST",body:JSON.stringify(formJSON(e.currentTarget))});setToken(out.token);await boot()}catch(err){$("#auth-error").textContent=err.message}});
$("#logout").addEventListener("click",()=>{setToken("");showAuth("login")});
$("#new-event").addEventListener("click",openEvent);$("#nav-add").addEventListener("click",openEvent);
$("#manage").addEventListener("click",openManage);$("#nav-more").addEventListener("click",()=>{if(state.me.role==="admin")openManage()});
$$("[data-close-event]").forEach(b=>b.addEventListener("click",()=>$("#event-dialog").close()));
$$("[data-close-manage]").forEach(b=>b.addEventListener("click",()=>$("#manage-dialog").close()));

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
  const f=e.currentTarget,fd=new FormData(f),reminder=fd.get("personal_reminder");
  const payload={title:fd.get("title"),calendar_id:fd.get("calendar_id"),starts_at:new Date(fd.get("starts_at")).toISOString(),ends_at:new Date(fd.get("ends_at")).toISOString(),all_day:false,location:fd.get("location"),notes:fd.get("notes"),assignee_ids:fd.getAll("assignee"),reminders:reminder?[{kind:"personal",provider:"android",minutes_before:Number(reminder),destination:""}]:[]};
  try{await api("/api/events",{method:"POST",body:JSON.stringify(payload)});$("#event-dialog").close();await loadEvents();render()}catch(err){$("#event-error").textContent=err.message}
});
boot().catch(()=>showAuth("login"));
