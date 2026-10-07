window.addEventListener("error",()=>showBootFailure());
window.addEventListener("unhandledrejection",()=>showBootFailure());

const $=s=>document.querySelector(s);
const $$=s=>document.querySelectorAll(s);
const savedDays=Number(localStorage.getItem("calden_view_days")||0);
const savedHidden=JSON.parse(localStorage.getItem("calden_hidden_calendars")||"[]");

const state={
  token:localStorage.getItem("calden_token")||"",
  me:null,settings:null,users:[],calendars:[],events:[],
  editingEvent:null,editingCalendar:null,editingUser:null,
  setupStep:0,currentPage:"calendar",
  viewDays:[1,7,14,30].includes(savedDays)?savedDays:7,
  anchorDate:startOfDay(new Date()),
  hiddenCalendars:new Set(Array.isArray(savedHidden)?savedHidden:[]),
  updateInfo:null,updatePoll:null
};

async function api(path,options={}){
  const headers={"Content-Type":"application/json",...(options.headers||{})};
  if(state.token)headers.Authorization=`Bearer ${state.token}`;
  const res=await fetch(path,{...options,headers});
  const body=res.status===204?null:await res.json().catch(()=>null);
  if(!res.ok)throw new Error(body?.error||`Request failed (${res.status})`);
  return body;
}
function setToken(token){state.token=token||"";if(token)localStorage.setItem("calden_token",token);else localStorage.removeItem("calden_token")}
function formJSON(form){return Object.fromEntries(new FormData(form).entries())}
function escapeHTML(v=""){return String(v).replace(/[&<>"']/g,c=>({"&":"&amp;","<":"&lt;",">":"&gt;",'"':"&quot;","'":"&#39;"}[c]))}
function escapeAttr(v=""){return escapeHTML(v)}
function safeColor(v){return /^#[0-9a-f]{6}$/i.test(v)?v:"#667085"}
function startOfDay(value){const d=new Date(value);d.setHours(0,0,0,0);return d}
function addDays(value,days){const d=new Date(value);d.setDate(d.getDate()+days);return d}
function sameDay(a,b){return startOfDay(a).getTime()===startOfDay(b).getTime()}
function localInput(d){const p=n=>String(n).padStart(2,"0");return `${d.getFullYear()}-${p(d.getMonth()+1)}-${p(d.getDate())}T${p(d.getHours())}:${p(d.getMinutes())}`}
function roleLabel(role){return role==="admin"?"Administrator":role==="restricted"?"Restricted member":"Family member"}
function formatDate(d,opts={month:"short",day:"numeric"}){return new Intl.DateTimeFormat(undefined,opts).format(d)}
function formatTime(d){return new Intl.DateTimeFormat(undefined,{hour:"numeric",minute:"2-digit"}).format(d)}
function clamp(n,min,max){return Math.min(max,Math.max(min,n))}

function showBootFailure(){
  const el=$("#boot-status");if(!el)return;
  el.classList.remove("hidden");el.classList.add("boot-error");
  el.textContent="CalDen could not finish loading. Refresh the page. If this continues, check the CalDen container logs.";
}
function hideBootStatus(){const el=$("#boot-status");if(el)el.classList.add("hidden")}

function showAuth(which){
  hideBootStatus();
  $("#app").classList.add("hidden");$("#auth").classList.remove("hidden");
  $("#setup-form").classList.toggle("hidden",which!=="setup");
  $("#login-form").classList.toggle("hidden",which!=="login");
  $("#auth-card").classList.toggle("setup-mode",which==="setup");
  $("#auth-error").textContent="";
  if(which==="setup"){
    const zone=Intl.DateTimeFormat().resolvedOptions().timeZone||"UTC";
    const form=$("#setup-form");form.timezone.value=zone;
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
    if(!localStorage.getItem("calden_view_days")&&[1,7,14,30].includes(Number(state.settings.default_view)))state.viewDays=Number(state.settings.default_view);
    await reloadSharedData();
    renderApp();
    navigate(state.currentPage,false);
  }catch(err){
    console.error(err);
    setToken("");showAuth("login");
  }
}

async function reloadSharedData(){
  [state.users,state.calendars]=await Promise.all([api("/api/users"),api("/api/calendars")]);
  await loadEvents();
}
async function loadEvents(){
  const from=addDays(state.anchorDate,-45),to=addDays(state.anchorDate,150);
  state.events=await api(`/api/events?from=${encodeURIComponent(from.toISOString())}&to=${encodeURIComponent(to.toISOString())}`);
}

function renderApp(){
  hideBootStatus();$("#auth").classList.add("hidden");$("#app").classList.remove("hidden");
  $("#household-label").textContent=state.settings?.household_name||"";
  $("#sidebar-name").textContent=state.me.display_name;
  $("#sidebar-role").textContent=roleLabel(state.me.role);
  $("#sidebar-avatar").textContent=state.me.initials||"?";
  $$(".admin-only").forEach(el=>el.classList.toggle("hidden",state.me.role!=="admin"));
  $(".admin-settings")?.classList.toggle("hidden",state.me.role!=="admin");
  $("#settings-account-name").textContent=state.me.display_name;
  $("#settings-account-username").textContent=state.me.username;
  $("#settings-account-role").textContent=roleLabel(state.me.role);
  renderEventControls();
  renderCalendar();
  renderAgenda();
  renderPeople();
  renderCalendars();
  renderSettings();
}

function navigate(page,load=true){
  const adminPages=new Set(["people","calendars","integrations","updates"]);
  if(adminPages.has(page)&&state.me?.role!=="admin")page="calendar";
  state.currentPage=page;
  $$(".app-page").forEach(el=>el.classList.toggle("hidden",el.id!==`page-${page}`));
  $$("[data-page]").forEach(el=>el.classList.toggle("active",el.dataset.page===page));
  const titles={calendar:"Calendar",agenda:"Agenda",people:"People",calendars:"Calendars",notifications:"Notifications",integrations:"Integrations",updates:"Updates",settings:"Settings"};
  $("#page-title").textContent=titles[page]||"CalDen";
  $("#new-event").classList.toggle("hidden",!["calendar","agenda"].includes(page));
  $("#sidebar").classList.remove("open");
  if(!load)return;
  if(page==="integrations")loadMonita();
  if(page==="updates")loadUpdater();
}

function renderEventControls(){
  $("#calendar-select").innerHTML='<option value="">Choose a calendar</option>'+state.calendars.filter(c=>c.can_edit).map(c=>`<option value="${c.id}">${escapeHTML(c.name)}</option>`).join("");
  $("#people-picker").innerHTML=state.users.filter(u=>u.active!==false).map(personChoice).join("");
}
function personChoice(u){return `<label class="person-check"><input type="checkbox" name="assignee" value="${u.id}"><span><i class="avatar">${escapeHTML(u.initials)}</i>${escapeHTML(u.display_name)}</span></label>`}

function visibleEvents(){
  return state.events.filter(e=>!state.hiddenCalendars.has(e.calendar_id));
}
function eventsForDay(day){
  const start=startOfDay(day),end=addDays(start,1);
  return visibleEvents().filter(e=>new Date(e.starts_at)<end&&new Date(e.ends_at)>=start);
}

function renderCalendar(){
  const label=$("#calendar-range-label");
  $$(".view-switcher button").forEach(b=>b.classList.toggle("active",Number(b.dataset.days)===state.viewDays));
  $("#calendar-strip").innerHTML=state.calendars.map(c=>{
    const hidden=state.hiddenCalendars.has(c.id);
    return `<button class="calendar-pill ${hidden?"calendar-hidden":""}" data-calendar-id="${c.id}" aria-pressed="${!hidden}">
      <span class="dot" style="--cal:${safeColor(c.color)}"></span><span>${escapeHTML(c.name)}</span>
    </button>`;
  }).join("");

  const start=startOfDay(state.anchorDate),end=addDays(start,state.viewDays-1);
  label.textContent=state.viewDays===1
    ? formatDate(start,{weekday:"long",month:"long",day:"numeric",year:"numeric"})
    : `${formatDate(start,{month:"short",day:"numeric"})} – ${formatDate(end,{month:"short",day:"numeric",year:"numeric"})}`;

  const host=$("#calendar-view");
  host.className=`calendar-view view-${state.viewDays}`;
  host.innerHTML=state.viewDays<=7?renderTimeline(state.viewDays):renderDayGrid(state.viewDays);
  bindCalendarEvents();
}

function renderTimeline(days){
  const start=startOfDay(state.anchorDate);
  const dayList=Array.from({length:days},(_,i)=>addDays(start,i));
  const hourStart=0,hourEnd=24,totalMinutes=(hourEnd-hourStart)*60;
  const header=dayList.map(day=>`<div class="timeline-day-header ${sameDay(day,new Date())?"today":""}"><span>${formatDate(day,{weekday:"short"})}</span><strong>${day.getDate()}</strong></div>`).join("");
  const lanes=dayList.map(day=>{
    const allDay=eventsForDay(day).filter(e=>e.all_day);
    const timed=eventsForDay(day).filter(e=>!e.all_day);
    const allDayHTML=allDay.map(e=>calendarEventBlock(e,true)).join("");
    const timedHTML=timed.map(e=>{
      const dayStart=startOfDay(day),dayEnd=addDays(dayStart,1);
      const start=new Date(Math.max(new Date(e.starts_at).getTime(),dayStart.getTime()));
      const end=new Date(Math.min(new Date(e.ends_at).getTime(),dayEnd.getTime()));
      const startMin=(start.getHours()*60+start.getMinutes())-hourStart*60;
      const endMin=Math.max(startMin+20,(end.getHours()*60+end.getMinutes())-hourStart*60);
      const top=clamp(startMin/totalMinutes*100,0,100);
      const height=clamp((endMin-startMin)/totalMinutes*100,1.4,100-top);
      return `<button class="timed-event" data-event-id="${e.id}" style="--cal:${safeColor(e.color)};--top:${top}%;--height:${height}%">
        <strong>${escapeHTML(e.title)}</strong><span>${formatTime(new Date(e.starts_at))}</span>${avatarMini(e)}
      </button>`;
    }).join("");
    return `<div class="timeline-lane"><div class="all-day-lane">${allDayHTML}</div><div class="timed-lane">${timedHTML}</div></div>`;
  }).join("");
  const hours=Array.from({length:24},(_,h)=>`<div class="hour-label" style="--hour:${h}">${h===0?"12 AM":h<12?`${h} AM`:h===12?"12 PM":`${h-12} PM`}</div>`).join("");
  return `<div class="timeline-wrap" style="--days:${days}">
    <div class="timeline-corner">All day</div><div class="timeline-headers">${header}</div>
    <div class="time-gutter">${hours}</div><div class="timeline-lanes">${lanes}</div>
  </div>`;
}

function renderDayGrid(days){
  const start=startOfDay(state.anchorDate);
  const cells=Array.from({length:days},(_,i)=>{
    const day=addDays(start,i),events=eventsForDay(day).sort((a,b)=>new Date(a.starts_at)-new Date(b.starts_at));
    const visible=events.slice(0,state.viewDays===30?4:6);
    const more=events.length-visible.length;
    return `<article class="day-cell ${sameDay(day,new Date())?"today":""}" data-date="${day.toISOString()}">
      <header><span>${formatDate(day,{weekday:"short"})}</span><strong>${day.getDate()}</strong></header>
      <div class="day-events">${visible.map(e=>calendarEventBlock(e,false)).join("")}${more>0?`<span class="more-events">+${more} more</span>`:""}</div>
    </article>`;
  }).join("");
  return `<div class="day-grid" style="--grid-days:7">${cells}</div>`;
}

function calendarEventBlock(e,compact=false){
  const time=e.all_day?"All day":formatTime(new Date(e.starts_at));
  return `<button class="calendar-event ${compact?"compact":""}" data-event-id="${e.id}" style="--cal:${safeColor(e.color)}">
    <span class="event-color"></span><span class="calendar-event-copy"><strong>${e.is_recurring?'<span class="repeat-mark" title="Repeating event">↻</span> ':""}${escapeHTML(e.title)}</strong><small>${escapeHTML(time)}</small></span>${avatarMini(e)}
  </button>`;
}
function avatarMini(e){
  const people=e.assignees||[];
  if(!people.length)return "";
  return `<span class="mini-avatars">${people.slice(0,3).map(p=>`<i title="${escapeAttr(p.display_name)}">${escapeHTML(p.initials)}</i>`).join("")}${people.length>3?`<i>+${people.length-3}</i>`:""}</span>`;
}
function bindCalendarEvents(){
  $("#calendar-view").querySelectorAll("[data-event-id]").forEach(el=>el.addEventListener("click",()=>{
    const ev=state.events.find(x=>x.id===el.dataset.eventId);if(ev)openEvent(ev);
  }));
}

function renderAgenda(){
  const query=($("#agenda-search")?.value||"").trim().toLowerCase();
  const events=visibleEvents().filter(e=>{
    if(!query)return true;
    return [e.title,e.location,e.notes,e.calendar_name,...(e.assignees||[]).map(a=>a.display_name)].join(" ").toLowerCase().includes(query);
  }).sort((a,b)=>new Date(a.starts_at)-new Date(b.starts_at));
  $("#event-count").textContent=`${events.length} event${events.length===1?"":"s"}`;
  $("#events").innerHTML=events.length?events.map(eventCard).join(""):'<div class="empty-state"><strong>No matching events</strong><span>Add an event or change your filters.</span></div>';
  $("#events").querySelectorAll("[data-event-id]").forEach(el=>el.addEventListener("click",()=>{
    const ev=state.events.find(x=>x.id===el.dataset.eventId);if(ev)openEvent(ev);
  }));
}
function eventCard(e){
  const start=new Date(e.starts_at),end=new Date(e.ends_at);
  return `<button class="agenda-event" data-event-id="${e.id}" style="--cal:${safeColor(e.color)}">
    <span class="agenda-color"></span><span class="agenda-date"><strong>${formatDate(start,{month:"short",day:"numeric"})}</strong><small>${e.all_day?"All day":formatTime(start)}</small></span>
    <span class="agenda-main"><strong>${e.is_recurring?'<span class="repeat-mark" title="Repeating event">↻</span> ':""}${escapeHTML(e.title)}</strong><small>${escapeHTML(e.calendar_name)}${e.location?" · "+escapeHTML(e.location):""}${e.all_day?"":` · ends ${escapeHTML(formatTime(end))}`}</small></span>
    ${avatarMini(e)}
  </button>`;
}

function renderPeople(){
  if(state.me.role!=="admin")return;
  $("#people-list").innerHTML=state.users.map(u=>`<article class="management-card ${u.active===false?"inactive":""}">
    <div class="management-avatar">${escapeHTML(u.initials)}</div>
    <div class="management-copy"><strong>${escapeHTML(u.display_name)}</strong><span>@${escapeHTML(u.username)}</span><small>${roleLabel(u.role)} · ${u.active===false?"Inactive":"Active"}</small></div>
    <button class="button secondary compact edit-person" type="button" data-user-id="${u.id}">Edit</button>
  </article>`).join("");
  $("#people-list").querySelectorAll(".edit-person").forEach(b=>b.addEventListener("click",()=>beginPersonEdit(b.dataset.userId)));
}

function resetPersonForm(){
  const form=$("#person-form");form.reset();state.editingUser=null;
  form.user_id.value="";form.username.disabled=false;form.active.checked=true;
  $("#person-active-row").classList.add("hidden");$("#cancel-person-edit").classList.add("hidden");
  $("#person-form-eyebrow").textContent="New person";$("#person-form-title").textContent="Add a family member";
  $("#person-password-label").firstChild.textContent="Temporary password";
  $("#person-error").textContent="";$("#person-status").textContent="";
}
function beginPersonEdit(id){
  const user=state.users.find(u=>u.id===id);if(!user)return;
  state.editingUser=user;const form=$("#person-form");
  form.user_id.value=user.id;form.display_name.value=user.display_name;form.username.value=user.username;form.username.disabled=true;
  form.password.value="";form.role.value=user.role;form.active.checked=user.active!==false;
  $("#person-active-row").classList.remove("hidden");$("#cancel-person-edit").classList.remove("hidden");
  $("#person-form-eyebrow").textContent="Edit person";$("#person-form-title").textContent=user.display_name;
  $("#person-password-label").firstChild.textContent="New password (leave blank to keep current)";
  $("#person-error").textContent="";$("#person-status").textContent="";
}

function renderCalendars(){
  if(state.me.role!=="admin")return;
  $("#calendar-list").innerHTML=state.calendars.map(c=>`<article class="management-card">
    <span class="calendar-swatch" style="--cal:${safeColor(c.color)}"></span>
    <div class="management-copy"><strong>${escapeHTML(c.name)}</strong><span>${escapeHTML(c.description||"No description")}</span><small>${c.can_edit?"Editable":"View only"}</small></div>
    <button class="button secondary compact edit-calendar" type="button" data-calendar-id="${c.id}">Edit</button>
  </article>`).join("")||'<div class="empty-state"><strong>No calendars</strong><span>Create your first calendar.</span></div>';
  $("#calendar-list").querySelectorAll(".edit-calendar").forEach(b=>b.addEventListener("click",()=>beginCalendarEdit(b.dataset.calendarId)));
  renderCalendarPermissionChecks();
}

function renderCalendarPermissionChecks(){
  const activeUsers=state.users.filter(u=>u.active!==false);
  const checks=activeUsers.map(u=>`<label class="plain-check"><input type="checkbox" value="${u.id}"><span>${escapeHTML(u.display_name)}</span></label>`).join("");
  $("#calendar-viewers").innerHTML=checks;$("#calendar-editors").innerHTML=checks;
}
function resetCalendarForm(){
  const form=$("#calendar-form");form.reset();form.color.value="#2f6fed";form.calendar_id.value="";state.editingCalendar=null;
  $("#calendar-form-eyebrow").textContent="New calendar";$("#calendar-form-title").textContent="Create a calendar";
  $("#delete-calendar").classList.add("hidden");$("#cancel-calendar-edit").classList.add("hidden");
  $("#calendar-error").textContent="";$("#calendar-status").textContent="";
  renderCalendarPermissionChecks();
  [...$("#calendar-viewers").querySelectorAll("input"),...$("#calendar-editors").querySelectorAll("input")].forEach(i=>{if(i.value===state.me.id)i.checked=true});
}
async function beginCalendarEdit(id){
  const cal=state.calendars.find(c=>c.id===id);if(!cal)return;
  state.editingCalendar=cal;const form=$("#calendar-form");
  form.calendar_id.value=cal.id;form.name.value=cal.name;form.color.value=cal.color;form.description.value=cal.description||"";
  $("#calendar-form-eyebrow").textContent="Edit calendar";$("#calendar-form-title").textContent=cal.name;
  $("#delete-calendar").classList.remove("hidden");$("#cancel-calendar-edit").classList.remove("hidden");
  $("#calendar-error").textContent="";$("#calendar-status").textContent="";
  renderCalendarPermissionChecks();
  try{
    const perms=await api(`/api/calendars/${id}/permissions`);
    const view=new Set(perms.filter(p=>p.can_view).map(p=>p.user_id));
    const edit=new Set(perms.filter(p=>p.can_edit).map(p=>p.user_id));
    $("#calendar-viewers").querySelectorAll("input").forEach(i=>i.checked=view.has(i.value));
    $("#calendar-editors").querySelectorAll("input").forEach(i=>i.checked=edit.has(i.value));
  }catch(err){$("#calendar-error").textContent=err.message}
}

function renderSettings(){
  const form=$("#general-settings-form");
  if(form){
    form.household_name.value=state.settings?.household_name||"";
    form.timezone.value=state.settings?.timezone||Intl.DateTimeFormat().resolvedOptions().timeZone||"UTC";
    form.week_start.value=state.settings?.week_start||"sunday";
    form.default_view.value=String(state.settings?.default_view||7);
  }
}

function updateRepeatUI(){
  const form=$("#event-form"),frequency=form.repeat_frequency.value,endType=form.repeat_end_type.value;
  $("#repeat-options").classList.toggle("hidden",!frequency);
  $("#repeat-weekdays").classList.toggle("hidden",frequency!=="weekly");
  $("#repeat-until-row").classList.toggle("hidden",!frequency||endType!=="date");
  $("#repeat-count-row").classList.toggle("hidden",!frequency||endType!=="count");
  const interval=Math.max(1,Number(form.repeat_interval.value)||1);
  const units={daily:"day",weekly:"week",monthly:"month",yearly:"year"};
  const unit=units[frequency]||"";
  $("#repeat-unit").textContent=unit+(interval===1?"":"s");
  if(frequency==="weekly"&&![...form.querySelectorAll('input[name="repeat_weekday"]')].some(i=>i.checked)){
    const start=form.starts_at.value?new Date(form.starts_at.value):new Date();
    const weekday=form.querySelector(`input[name="repeat_weekday"][value="${start.getDay()}"]`);
    if(weekday)weekday.checked=true;
  }
}

function recurrencePayload(form){
  const frequency=form.repeat_frequency.value;
  if(!frequency)return null;
  const rule={
    frequency,
    interval:Math.max(1,Number(form.repeat_interval.value)||1),
    weekdays:frequency==="weekly"?[...form.querySelectorAll('input[name="repeat_weekday"]:checked')].map(i=>Number(i.value)):[],
    until:null,
    occurrence_count:null
  };
  if(form.repeat_end_type.value==="date"&&form.repeat_until.value){
    rule.until=new Date(form.repeat_until.value+"T23:59:59").toISOString();
  }
  if(form.repeat_end_type.value==="count"){
    rule.occurrence_count=Math.max(1,Number(form.repeat_count.value)||1);
  }
  return rule;
}

function openEvent(existing=null,dateHint=null){
  if(!state.calendars.some(c=>c.can_edit)){
    if(state.me.role==="admin"){navigate("calendars");return}
    alert("You do not have a calendar you can add events to yet.");return;
  }
  const form=$("#event-form");form.reset();state.editingEvent=existing;
  $("#event-dialog-title").textContent=existing?"Edit event":"Add event";$("#delete-event").classList.toggle("hidden",!existing);
  if(existing){
    form.event_id.value=existing.id;form.title.value=existing.title;form.calendar_id.value=existing.calendar_id;
    form.starts_at.value=localInput(new Date(existing.series_starts_at||existing.starts_at));
    form.ends_at.value=localInput(new Date(existing.series_ends_at||existing.ends_at));
    form.all_day.checked=!!existing.all_day;form.location.value=existing.location||"";form.notes.value=existing.notes||"";
    const recurrence=existing.recurrence||null;
    form.repeat_frequency.value=recurrence?.frequency||"";
    form.repeat_interval.value=String(recurrence?.interval||1);
    [...form.querySelectorAll('input[name="repeat_weekday"]')].forEach(i=>i.checked=(recurrence?.weekdays||[]).includes(Number(i.value)));
    if(recurrence?.until){
      form.repeat_end_type.value="date";
      const untilDate=new Date(recurrence.until);
      form.repeat_until.value=`${untilDate.getFullYear()}-${String(untilDate.getMonth()+1).padStart(2,"0")}-${String(untilDate.getDate()).padStart(2,"0")}`;
    }else if(recurrence?.occurrence_count){
      form.repeat_end_type.value="count";form.repeat_count.value=String(recurrence.occurrence_count);
    }else{
      form.repeat_end_type.value="never";
    }
    const personal=(existing.reminders||[]).find(r=>r.kind==="personal"&&r.provider==="android");
    const system=(existing.reminders||[]).find(r=>r.kind==="system"&&r.provider==="monita");
    form.personal_reminder.value=personal?String(personal.minutes_before):"";
    form.system_reminder_enabled.checked=!!system;form.system_reminder.value=system?String(system.minutes_before):"1440";
    [...form.querySelectorAll('input[name="assignee"]')].forEach(i=>i.checked=(existing.assignees||[]).some(a=>a.id===i.value));
  }else{
    const start=dateHint?new Date(dateHint):new Date(Date.now()+3600000);start.setMinutes(0,0,0);
    if(dateHint&&start.getHours()===0)start.setHours(9);
    const end=new Date(start.getTime()+3600000);
    form.starts_at.value=localInput(start);form.ends_at.value=localInput(end);form.system_reminder.value="1440";
    form.repeat_frequency.value="";form.repeat_interval.value="1";form.repeat_end_type.value="never";form.repeat_count.value="10";
    [...form.querySelectorAll('input[name="repeat_weekday"]')].forEach(i=>i.checked=false);
  }
  updateRepeatUI();
  $("#system-reminder-time").classList.toggle("hidden",!form.system_reminder_enabled.checked);
  $("#event-error").textContent="";$("#event-dialog").showModal();
}

async function loadMonita(){
  if(state.me.role!=="admin")return;
  try{
    const m=await api("/api/integrations/monita"),form=$("#monita-form");
    form.enabled.checked=!!m.enabled;form.server_url.value=m.server_url||"";form.token.value=m.token||"";form.default_channel.value=m.default_channel||"";
  }catch(err){$("#monita-status").textContent=err.message}
}

async function loadUpdater(){
  if(state.me.role!=="admin")return;
  $("#update-state").textContent="Checking for updates…";
  try{
    const info=await api("/api/system/update");state.updateInfo=info;renderUpdater(info);
    if(info.status&&["backup","preparing","downloading","verifying","installing","restarting"].includes(info.status.state))startUpdatePolling();
  }catch(err){$("#update-state").textContent=err.message}
}
function renderUpdater(info){
  const latest=info.latest;
  $("#update-current").textContent=info.current_version||"Unknown";
  $("#update-latest").textContent=latest?.version||"None on this channel";
  $("#update-channel").value=info.preferences?.channel||"stable";
  $("#update-auto-check").checked=info.preferences?.auto_check!==false;
  const status=info.status||{state:"idle",progress:0,activity:[]};
  const active=["backup","preparing","downloading","verifying","installing","restarting"].includes(status.state);
  $("#update-progress-wrap").classList.toggle("hidden",!active&&status.state!=="completed"&&status.state!=="failed");
  $("#update-progress").style.width=`${clamp(Number(status.progress)||0,0,100)}%`;
  $("#update-progress-label").textContent=`${Number(status.progress)||0}%`;
  $("#update-state").className=`update-state state-${status.state||"idle"}`;
  if(active||status.state==="completed"||status.state==="failed")$("#update-state").textContent=status.message||status.step||status.state;
  else if(info.available)$("#update-state").textContent=`CalDen ${latest.version} is available.`;
  else $("#update-state").textContent="CalDen is up to date for this channel.";
  $("#apply-update").classList.toggle("hidden",!info.available||active);
  $("#apply-update").dataset.version=latest?.version||"";
  $("#rollback-update").classList.toggle("hidden",!status.rollback_ready||active);
  $("#update-release-notes").innerHTML=latest?`<h3>${escapeHTML(latest.name||"Release "+latest.version)}</h3><p class="release-meta">${latest.published_at?escapeHTML(formatDate(new Date(latest.published_at),{month:"long",day:"numeric",year:"numeric"})):""}</p><div>${escapeHTML(latest.notes||"No release notes were provided.").replace(/\n/g,"<br>")}</div>`:'<p class="muted">No eligible release was found for this channel.</p>';
  $("#update-activity").innerHTML=(status.activity||[]).slice().reverse().map(a=>`<div><span>${escapeHTML(formatTime(new Date(a.timestamp)))}</span><strong>${escapeHTML(a.message)}</strong></div>`).join("");
}
function startUpdatePolling(){
  clearInterval(state.updatePoll);
  state.updatePoll=setInterval(async()=>{
    try{
      const status=await api("/api/system/update/status");
      if(state.updateInfo){state.updateInfo.status=status;renderUpdater(state.updateInfo)}
      if(!["backup","preparing","downloading","verifying","installing","restarting"].includes(status.state)){
        clearInterval(state.updatePoll);state.updatePoll=null;setTimeout(loadUpdater,600);
      }
    }catch{}
  },1000);
}

function setSetupStep(step){
  state.setupStep=Math.max(0,Math.min(3,step));
  $$(".setup-step").forEach(s=>s.classList.toggle("hidden",Number(s.dataset.step)!==state.setupStep));
  $$(".setup-dot").forEach(d=>d.classList.toggle("active",Number(d.dataset.dot)<=state.setupStep));
  $("#setup-back").classList.toggle("hidden",state.setupStep===0);$("#setup-next").classList.toggle("hidden",state.setupStep===3);$("#setup-finish").classList.toggle("hidden",state.setupStep!==3);
  if(state.setupStep===3)updateSetupSummary();
}
function setupStepValid(){
  const form=$("#setup-form");$("#auth-error").textContent="";
  if(state.setupStep===0&&!form.household_name.value.trim()){ $("#auth-error").textContent="Give your family calendar a name.";form.household_name.focus();return false }
  if(state.setupStep===1){
    if(!form.display_name.value.trim()){ $("#auth-error").textContent="Enter your name.";form.display_name.focus();return false }
    if(form.username.value.trim().length<3){ $("#auth-error").textContent="Username must be at least 3 characters.";form.username.focus();return false }
    if(form.password.value.length<8){ $("#auth-error").textContent="Password must be at least 8 characters.";form.password.focus();return false }
    if(form.password.value!==form.confirm_password.value){ $("#auth-error").textContent="Those passwords do not match.";form.confirm_password.focus();return false }
  }
  return true;
}
function updateSetupSummary(){
  const form=$("#setup-form");
  const names=[...form.querySelectorAll('input[name="starter_calendar"]:checked')].map(i=>i.closest("label").querySelector("b").textContent);
  $("#summary-household").textContent=form.household_name.value.trim();$("#summary-admin").textContent=form.display_name.value.trim();
  $("#summary-timezone").textContent=(form.timezone.value||"UTC").replaceAll("_"," ");$("#summary-calendars").textContent=names.length?names.join(", "):"Family";
}

$("#setup-next").addEventListener("click",()=>{if(setupStepValid())setSetupStep(state.setupStep+1)});
$("#setup-back").addEventListener("click",()=>setSetupStep(state.setupStep-1));
$("#setup-form").addEventListener("submit",async e=>{
  e.preventDefault();if(!setupStepValid())return;const form=e.currentTarget;
  $("#auth-error").textContent="";$("#setup-finish").disabled=true;$("#setup-finish").textContent="Setting up…";
  const payload={household_name:form.household_name.value.trim(),timezone:form.timezone.value||"UTC",display_name:form.display_name.value.trim(),username:form.username.value.trim(),password:form.password.value,starter_calendars:[...form.querySelectorAll('input[name="starter_calendar"]:checked')].map(i=>i.value)};
  try{const out=await api("/api/setup",{method:"POST",body:JSON.stringify(payload)});setToken(out.token);await boot()}
  catch(err){$("#auth-error").textContent=err.message}
  finally{$("#setup-finish").disabled=false;$("#setup-finish").textContent="Finish setup"}
});
$("#login-form").addEventListener("submit",async e=>{
  e.preventDefault();const form=e.currentTarget;$("#auth-error").textContent="";
  try{const out=await api("/api/login",{method:"POST",body:JSON.stringify(formJSON(form))});setToken(out.token);await boot()}
  catch(err){$("#auth-error").textContent=err.message}
});
$("#logout").addEventListener("click",()=>{setToken("");location.reload()});
$$("[data-page]").forEach(b=>b.addEventListener("click",()=>navigate(b.dataset.page)));
$("#mobile-menu").addEventListener("click",()=>$("#sidebar").classList.toggle("open"));
$("#new-event").addEventListener("click",()=>openEvent());
$("#nav-add").addEventListener("click",()=>openEvent());
$$("#calendar-strip").forEach(()=>{});
$("#calendar-strip").addEventListener("click",e=>{
  const b=e.target.closest("[data-calendar-id]");if(!b)return;
  const id=b.dataset.calendarId;if(state.hiddenCalendars.has(id))state.hiddenCalendars.delete(id);else state.hiddenCalendars.add(id);
  localStorage.setItem("calden_hidden_calendars",JSON.stringify([...state.hiddenCalendars]));renderCalendar();renderAgenda();
});
$$(".view-switcher button").forEach(b=>b.addEventListener("click",()=>{
  state.viewDays=Number(b.dataset.days);localStorage.setItem("calden_view_days",String(state.viewDays));renderCalendar();
}));
$("#calendar-prev").addEventListener("click",()=>{state.anchorDate=addDays(state.anchorDate,-state.viewDays);renderCalendar()});
$("#calendar-next").addEventListener("click",()=>{state.anchorDate=addDays(state.anchorDate,state.viewDays);renderCalendar()});
$("#calendar-today").addEventListener("click",()=>{state.anchorDate=startOfDay(new Date());renderCalendar()});
$("#calendar-view").addEventListener("dblclick",e=>{
  const cell=e.target.closest(".day-cell");if(cell)openEvent(null,new Date(cell.dataset.date));
});
$("#agenda-search").addEventListener("input",renderAgenda);
$$("#event-dialog [data-close-event]").forEach(b=>b.addEventListener("click",()=>$("#event-dialog").close()));
$("#event-form").system_reminder_enabled.addEventListener("change",e=>$("#system-reminder-time").classList.toggle("hidden",!e.target.checked));
$("#event-form").repeat_frequency.addEventListener("change",updateRepeatUI);
$("#event-form").repeat_interval.addEventListener("input",updateRepeatUI);
$("#event-form").repeat_end_type.addEventListener("change",updateRepeatUI);
$("#event-form").starts_at.addEventListener("change",()=>{if($("#event-form").repeat_frequency.value==="weekly")updateRepeatUI()});
$("#delete-event").addEventListener("click",async()=>{
  if(!state.editingEvent||!confirm("Delete this event?"))return;
  try{await api("/api/events/"+state.editingEvent.id,{method:"DELETE"});$("#event-dialog").close();state.editingEvent=null;await loadEvents();renderCalendar();renderAgenda()}
  catch(err){$("#event-error").textContent=err.message}
});
$("#event-form").addEventListener("submit",async e=>{
  e.preventDefault();const form=e.currentTarget,fd=new FormData(form);$("#event-error").textContent="";
  const reminders=[],personal=fd.get("personal_reminder");
  if(personal)reminders.push({kind:"personal",provider:"android",minutes_before:Number(personal),destination:""});
  if(form.system_reminder_enabled.checked)reminders.push({kind:"system",provider:"monita",minutes_before:Number(fd.get("system_reminder")||1440),destination:""});
  const payload={title:fd.get("title"),calendar_id:fd.get("calendar_id"),starts_at:new Date(fd.get("starts_at")).toISOString(),ends_at:new Date(fd.get("ends_at")).toISOString(),all_day:form.all_day.checked,location:fd.get("location"),notes:fd.get("notes"),assignee_ids:fd.getAll("assignee"),reminders,recurrence:recurrencePayload(form)};
  const target=state.editingEvent?"/api/events/"+state.editingEvent.id:"/api/events",method=state.editingEvent?"PUT":"POST";
  try{await api(target,{method,body:JSON.stringify(payload)});$("#event-dialog").close();state.editingEvent=null;await loadEvents();renderCalendar();renderAgenda()}
  catch(err){$("#event-error").textContent=err.message}
});

$("#person-form").addEventListener("submit",async e=>{
  e.preventDefault();const form=e.currentTarget;$("#person-error").textContent="";$("#person-status").textContent="";
  const editing=state.editingUser;
  try{
    if(editing){
      const payload={display_name:form.display_name.value.trim(),role:form.role.value,active:form.active.checked,password:form.password.value};
      await api("/api/users/"+editing.id,{method:"PUT",body:JSON.stringify(payload)});
      $("#person-status").textContent="Person updated.";
    }else{
      const payload={display_name:form.display_name.value.trim(),username:form.username.value.trim(),role:form.role.value,password:form.password.value};
      await api("/api/users",{method:"POST",body:JSON.stringify(payload)});
      $("#person-status").textContent="Person added.";
    }
    state.users=await api("/api/users");resetPersonForm();renderPeople();renderCalendarPermissionChecks();renderEventControls();
  }catch(err){$("#person-error").textContent=err.message}
});
$("#cancel-person-edit").addEventListener("click",resetPersonForm);

$("#calendar-form").addEventListener("submit",async e=>{
  e.preventDefault();const form=e.currentTarget,fd=new FormData(form);$("#calendar-error").textContent="";$("#calendar-status").textContent="";
  const visible=[...$("#calendar-viewers").querySelectorAll("input:checked")].map(i=>i.value);
  const editors=[...$("#calendar-editors").querySelectorAll("input:checked")].map(i=>i.value);
  editors.forEach(id=>{if(!visible.includes(id))visible.push(id)});
  if(!visible.includes(state.me.id))visible.push(state.me.id);if(!editors.includes(state.me.id))editors.push(state.me.id);
  const payload={name:fd.get("name"),color:fd.get("color"),icon:"calendar",description:fd.get("description")};
  try{
    if(state.editingCalendar){
      const id=state.editingCalendar.id;
      await api("/api/calendars/"+id,{method:"PUT",body:JSON.stringify(payload)});
      await api("/api/calendars/"+id+"/permissions",{method:"PUT",body:JSON.stringify({visible_to:visible,editable_by:editors})});
      $("#calendar-status").textContent="Calendar updated.";
    }else{
      await api("/api/calendars",{method:"POST",body:JSON.stringify({...payload,visible_to:visible,editable_by:editors})});
      $("#calendar-status").textContent="Calendar created.";
    }
    state.calendars=await api("/api/calendars");resetCalendarForm();renderCalendars();renderCalendar();renderEventControls();
  }catch(err){$("#calendar-error").textContent=err.message}
});
$("#cancel-calendar-edit").addEventListener("click",resetCalendarForm);
$("#delete-calendar").addEventListener("click",async()=>{
  if(!state.editingCalendar||!confirm(`Delete "${state.editingCalendar.name}"? The calendar must be empty first.`))return;
  try{await api("/api/calendars/"+state.editingCalendar.id,{method:"DELETE"});state.calendars=await api("/api/calendars");resetCalendarForm();renderCalendars();renderCalendar();renderEventControls()}
  catch(err){$("#calendar-error").textContent=err.message}
});

$("#general-settings-form").addEventListener("submit",async e=>{
  e.preventDefault();const form=e.currentTarget;if(state.me.role!=="admin")return;
  $("#general-settings-status").textContent="";
  const payload={household_name:form.household_name.value.trim(),timezone:form.timezone.value.trim(),week_start:form.week_start.value,default_view:Number(form.default_view.value)};
  try{state.settings=await api("/api/settings/general",{method:"PUT",body:JSON.stringify(payload)});$("#household-label").textContent=state.settings.household_name;$("#general-settings-status").textContent="Household settings saved."}
  catch(err){$("#general-settings-status").textContent=err.message}
});

$("#monita-form").addEventListener("submit",async e=>{
  e.preventDefault();const form=e.currentTarget;$("#monita-status").textContent="";
  const payload={enabled:form.enabled.checked,server_url:form.server_url.value,token:form.token.value,default_channel:form.default_channel.value};
  try{await api("/api/integrations/monita",{method:"PUT",body:JSON.stringify(payload)});$("#monita-status").textContent="Monita settings saved."}
  catch(err){$("#monita-status").textContent=err.message}
});
$("#test-monita").addEventListener("click",async()=>{
  $("#monita-status").textContent="Sending test…";
  try{await api("/api/integrations/monita/test",{method:"POST",body:"{}"});$("#monita-status").textContent="Test reminder sent."}
  catch(err){$("#monita-status").textContent=err.message}
});

$("#check-updates").addEventListener("click",loadUpdater);
$("#save-update-prefs").addEventListener("click",async()=>{
  $("#update-pref-status").textContent="";
  try{
    await api("/api/system/update/preferences",{method:"PUT",body:JSON.stringify({channel:$("#update-channel").value,auto_check:$("#update-auto-check").checked})});
    $("#update-pref-status").textContent="Update preferences saved.";await loadUpdater();
  }catch(err){$("#update-pref-status").textContent=err.message}
});
$("#apply-update").addEventListener("click",async()=>{
  const version=$("#apply-update").dataset.version;if(!version)return;
  if(!confirm(`Install CalDen ${version}? CalDen will create a database safety backup and restart itself.`))return;
  try{const status=await api("/api/system/update/install",{method:"POST",body:JSON.stringify({version})});state.updateInfo.status=status;renderUpdater(state.updateInfo);startUpdatePolling()}
  catch(err){$("#update-state").textContent=err.message}
});
$("#rollback-update").addEventListener("click",async()=>{
  if(!confirm("Roll back to the previous CalDen runtime? CalDen will restart."))return;
  try{const status=await api("/api/system/update/rollback",{method:"POST",body:"{}"});state.updateInfo.status=status;renderUpdater(state.updateInfo);startUpdatePolling()}
  catch(err){$("#update-state").textContent=err.message}
});

resetPersonForm();
resetCalendarForm();
boot().catch(err=>{console.error(err);showBootFailure()});
