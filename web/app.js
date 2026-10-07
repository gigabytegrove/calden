window.addEventListener("error",()=>showBootFailure());
window.addEventListener("unhandledrejection",()=>showBootFailure());

const $=s=>document.querySelector(s);
const $$=s=>document.querySelectorAll(s);
const savedDays=Number(localStorage.getItem("calden_view_days")||0);
const savedHidden=JSON.parse(localStorage.getItem("calden_hidden_calendars")||"[]");
const savedDefaultCalendar=localStorage.getItem("calden_default_calendar")||"";
const savedDefaultDuration=Number(localStorage.getItem("calden_default_duration")||60);
const savedScrollNow=localStorage.getItem("calden_scroll_now")!=="false";

const state={
  token:localStorage.getItem("calden_token")||"",
  me:null,settings:null,users:[],calendars:[],categories:[],events:[],
  editingEvent:null,editingScope:"series",editingCalendar:null,editingCategory:null,editingUser:null,
  setupStep:0,currentPage:"calendar",
  viewDays:[1,7,14,30].includes(savedDays)?savedDays:7,
  anchorDate:startOfDay(new Date()),
  hiddenCalendars:new Set(Array.isArray(savedHidden)?savedHidden:[]),
  filters:{category:"",person:"",query:""},
  defaultCalendar:savedDefaultCalendar,
  defaultDuration:[30,60,90,120].includes(savedDefaultDuration)?savedDefaultDuration:60,
  scrollNow:savedScrollNow,
  settingsTab:"general",
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
async function apiForm(path,formData,method="POST"){
  const headers={};
  if(state.token)headers.Authorization=`Bearer ${state.token}`;
  const res=await fetch(path,{method,headers,body:formData});
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
function avatarMarkup(user,sizeClass=""){
  if(user?.avatar_url){
    return `<img class="avatar-image ${sizeClass}" src="${escapeAttr(user.avatar_url)}" alt="">`;
  }
  return `<span class="avatar-fallback ${sizeClass}">${escapeHTML(user?.initials||"?")}</span>`;
}
function setAvatarPreview(el,user){
  if(!el)return;
  el.innerHTML=user?.avatar_url
    ?`<img src="${escapeAttr(user.avatar_url)}" alt="">`
    :`<span>${escapeHTML(user?.initials||"?")}</span>`;
}
function formatDate(d,opts={month:"short",day:"numeric"}){return new Intl.DateTimeFormat(undefined,opts).format(d)}
function formatTime(d){return new Intl.DateTimeFormat(undefined,{hour:"numeric",minute:"2-digit"}).format(d)}
function clamp(n,min,max){return Math.min(max,Math.max(min,n))}
function eventKey(e){return `${e.id}|${e.occurrence_start||e.starts_at}`}
function findEventByKey(key){return state.events.find(e=>eventKey(e)===key)}

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
  [state.users,state.calendars,state.categories]=await Promise.all([
    api("/api/users"),api("/api/calendars"),api("/api/categories")
  ]);
  await loadEvents();
}
async function loadEvents(){
  const from=startOfDay(addDays(state.anchorDate,-30)),to=addDays(from,365);
  state.events=await api(`/api/events?from=${encodeURIComponent(from.toISOString())}&to=${encodeURIComponent(to.toISOString())}`);
}

function renderApp(){
  hideBootStatus();$("#auth").classList.add("hidden");$("#app").classList.remove("hidden");
  $("#household-label").textContent=state.settings?.household_name||"";
  $("#sidebar-name").textContent=state.me.display_name;
  $("#sidebar-role").textContent=roleLabel(state.me.role);
  $("#sidebar-avatar").innerHTML=state.me.avatar_url?`<img src="${escapeAttr(state.me.avatar_url)}" alt="">`:escapeHTML(state.me.initials||"?");
  $$(".admin-only").forEach(el=>el.classList.toggle("hidden",state.me.role!=="admin"));
  $$(".admin-settings").forEach(el=>el.classList.toggle("hidden",state.me.role!=="admin"));
  $("#settings-account-name").textContent=state.me.display_name;
  $("#settings-account-username").textContent=state.me.username;
  $("#settings-account-role").textContent=roleLabel(state.me.role);
  setAvatarPreview($("#settings-avatar-preview"),state.me);
  $("#remove-settings-avatar")?.classList.toggle("hidden",!state.me.avatar_url);
  renderEventControls();
  renderCalendar();
  renderAgenda();
  renderPeople();
  renderCalendars();
  renderCategories();
  renderNotifications();
  renderSettings();
}

function navigate(page,load=true){
  const adminPages=new Set(["people","calendars","categories","integrations","updates","backups","activity"]);
  if(adminPages.has(page)&&state.me?.role!=="admin")page="calendar";
  state.currentPage=page;
  $$(".app-page").forEach(el=>el.classList.toggle("hidden",el.id!==`page-${page}`));
  $$("[data-page]").forEach(el=>el.classList.toggle("active",el.dataset.page===page));
  const titles={calendar:"Calendar",agenda:"Agenda",people:"People",calendars:"Calendars",categories:"Categories",notifications:"Notifications",integrations:"Integrations",updates:"Updates",backups:"Backups & Restore",activity:"Activity",settings:"Settings"};
  $("#page-title").textContent=titles[page]||"CalDen";
  $("#new-event").classList.toggle("hidden",!["calendar","agenda"].includes(page));
  $("#sidebar").classList.remove("open");
  if(!load)return;
  if(page==="notifications")renderNotifications();
  if(page==="integrations")loadMonita();
  if(page==="updates")loadUpdater();
  if(page==="backups")loadBackups();
  if(page==="activity")loadActivity();
}

function renderEventControls(){
  const editable=state.calendars.filter(c=>c.can_edit);
  $("#calendar-select").innerHTML='<option value="">Choose a calendar</option>'+editable.map(c=>`<option value="${c.id}">${escapeHTML(c.name)}</option>`).join("");
  $("#category-select").innerHTML='<option value="">No category</option>'+state.categories.map(cat=>`<option value="${cat.id}">${escapeHTML(cat.name)}</option>`).join("");
  $("#people-picker").innerHTML=state.users.filter(u=>u.active!==false).map(personChoice).join("");

  const categoryFilter=$("#calendar-category-filter");
  if(categoryFilter){
    categoryFilter.innerHTML='<option value="">All categories</option>'+state.categories.map(cat=>`<option value="${cat.id}">${escapeHTML(cat.name)}</option>`).join("");
    categoryFilter.value=state.filters.category;
  }
  const personFilter=$("#calendar-person-filter");
  if(personFilter){
    personFilter.innerHTML='<option value="">Everyone</option>'+state.users.filter(u=>u.active!==false).map(u=>`<option value="${u.id}">${escapeHTML(u.display_name)}</option>`).join("");
    personFilter.value=state.filters.person;
  }

  const defaultCalendar=$("#settings-default-calendar");
  if(defaultCalendar){
    defaultCalendar.innerHTML='<option value="">First editable calendar</option>'+editable.map(cal=>`<option value="${cal.id}">${escapeHTML(cal.name)}</option>`).join("");
    if(!editable.some(cal=>cal.id===state.defaultCalendar))state.defaultCalendar="";
    defaultCalendar.value=state.defaultCalendar;
  }
}
function personChoice(u){
  return `<label class="person-check"><input type="checkbox" name="assignee" value="${u.id}"><span><i class="avatar">${avatarMarkup(u)}</i>${escapeHTML(u.display_name)}</span></label>`;
}

function calendarViewStart(){
  const anchor=startOfDay(state.anchorDate);
  if(state.viewDays===1)return anchor;
  const firstDay=state.settings?.week_start==="monday"?1:0;
  const offset=(anchor.getDay()-firstDay+7)%7;
  return addDays(anchor,-offset);
}
function eventMatchesFilters(e){
  if(state.hiddenCalendars.has(e.calendar_id))return false;
  if(state.filters.category&&e.category_id!==state.filters.category)return false;
  if(state.filters.person&&!(e.assignees||[]).some(a=>a.id===state.filters.person))return false;
  if(state.filters.query){
    const haystack=[
      e.title,e.location,e.notes,e.calendar_name,e.category_name,
      ...(e.assignees||[]).map(a=>a.display_name)
    ].filter(Boolean).join(" ").toLowerCase();
    if(!haystack.includes(state.filters.query.toLowerCase()))return false;
  }
  return true;
}
function visibleEvents(){return state.events.filter(eventMatchesFilters)}
function eventsForDay(day){
  const start=startOfDay(day),end=addDays(start,1);
  return visibleEvents().filter(e=>new Date(e.starts_at)<end&&new Date(e.ends_at)>=start);
}
function renderCalendarFilters(){
  const category=$("#calendar-category-filter"),person=$("#calendar-person-filter"),search=$("#calendar-search");
  if(category)category.value=state.filters.category;
  if(person)person.value=state.filters.person;
  if(search&&search.value!==state.filters.query)search.value=state.filters.query;
  $("#clear-calendar-filters")?.classList.toggle("hidden",!state.filters.category&&!state.filters.person&&!state.filters.query);
}

function renderCalendar(){
  const label=$("#calendar-range-label");
  $$$(".view-switcher button").forEach(b=>b.classList.toggle("active",Number(b.dataset.days)===state.viewDays));
  $("#calendar-strip").innerHTML=state.calendars.map(cal=>{
    const hidden=state.hiddenCalendars.has(cal.id);
    return `<button class="calendar-pill ${hidden?"calendar-hidden":""}" data-calendar-id="${cal.id}" aria-pressed="${!hidden}">
      <span class="dot" style="--cal:${safeColor(cal.color)}"></span><span>${escapeHTML(cal.name)}</span>
    </button>`;
  }).join("");
  renderCalendarFilters();

  const start=calendarViewStart(),end=addDays(start,state.viewDays-1);
  label.textContent=state.viewDays===1
    ?formatDate(start,{weekday:"long",month:"long",day:"numeric",year:"numeric"})
    :`${formatDate(start,{month:"short",day:"numeric"})} – ${formatDate(end,{month:"short",day:"numeric",year:"numeric"})}`;

  const host=$("#calendar-view");
  host.className=`calendar-view view-${state.viewDays}`;
  host.innerHTML=state.viewDays<=7?renderTimeline(state.viewDays):renderDayGrid(state.viewDays);
  bindCalendarEvents();
  scrollCalendarNearNow();
}

function layoutTimedEvents(events,day){
  const dayStart=startOfDay(day),dayEnd=addDays(dayStart,1);
  const items=events.map(event=>{
    const clippedStart=new Date(Math.max(new Date(event.starts_at).getTime(),dayStart.getTime()));
    const clippedEnd=new Date(Math.min(new Date(event.ends_at).getTime(),dayEnd.getTime()));
    const startMin=(clippedStart.getTime()-dayStart.getTime())/60000;
    const rawEndMin=(clippedEnd.getTime()-dayStart.getTime())/60000;
    return {
      event,
      startMin,
      endMin:Math.max(startMin+15,rawEndMin)
    };
  }).sort((a,b)=>a.startMin-b.startMin||a.endMin-b.endMin);

  const result=[];
  let group=[],groupEnd=-1;
  const flush=()=>{
    if(!group.length)return;
    const columnEnds=[];
    group.forEach(item=>{
      let col=columnEnds.findIndex(end=>end<=item.startMin);
      if(col<0)col=columnEnds.length;
      columnEnds[col]=item.endMin;
      item.column=col;
    });
    const columns=Math.max(1,columnEnds.length);
    group.forEach(item=>{item.columns=columns;result.push(item)});
    group=[];
  };
  items.forEach(item=>{
    if(group.length&&item.startMin>=groupEnd){flush();groupEnd=-1}
    group.push(item);groupEnd=Math.max(groupEnd,item.endMin);
  });
  flush();
  return result;
}

function renderTimeline(days){
  const start=calendarViewStart();
  const dayList=Array.from({length:days},(_,i)=>addDays(start,i));
  const totalMinutes=24*60;
  const header=dayList.map(day=>`<div class="timeline-day-header ${sameDay(day,new Date())?"today":""}"><span>${formatDate(day,{weekday:"short"})}</span><strong>${day.getDate()}</strong></div>`).join("");
  const lanes=dayList.map(day=>{
    const allDay=eventsForDay(day).filter(e=>e.all_day);
    const timed=eventsForDay(day).filter(e=>!e.all_day);
    const allDayHTML=allDay.map(e=>calendarEventBlock(e,true)).join("");
    const timedHTML=layoutTimedEvents(timed,day).map(item=>{
      const top=clamp(item.startMin/totalMinutes*100,0,100);
      const height=clamp((item.endMin-item.startMin)/totalMinutes*100,1.4,100-top);
      const left=item.column/item.columns*100,width=100/item.columns;
      const e=item.event;
      return `<button class="timed-event" data-event-key="${escapeAttr(eventKey(e))}" style="--cal:${safeColor(e.calendar_color||e.color)};--top:${top}%;--height:${height}%;--left:${left}%;--width:${width}%">
        <strong>${escapeHTML(e.title)}</strong><span>${formatTime(new Date(e.starts_at))}${e.category_name?" · "+escapeHTML(e.category_name):""}</span>${avatarMini(e)}
      </button>`;
    }).join("");
    const now=new Date(),nowMinutes=now.getHours()*60+now.getMinutes();
    const nowLine=sameDay(day,now)?`<div class="current-time-line" style="--now:${nowMinutes/totalMinutes*100}%"><i></i></div>`:"";
    return `<div class="timeline-lane"><div class="all-day-lane">${allDayHTML}</div><div class="timed-lane" data-day="${escapeAttr(day.toISOString())}">${timedHTML}${nowLine}</div></div>`;
  }).join("");
  const hours=Array.from({length:24},(_,h)=>`<div class="hour-label" style="--hour:${h}">${h===0?"12 AM":h<12?`${h} AM`:h===12?"12 PM":`${h-12} PM`}</div>`).join("");
  return `<div class="timeline-wrap" style="--days:${days}">
    <div class="timeline-corner">All day</div><div class="timeline-headers">${header}</div>
    <div class="time-gutter">${hours}</div><div class="timeline-lanes">${lanes}</div>
  </div>`;
}

function renderDayGrid(days){
  const start=calendarViewStart();
  const weekdayHeader=Array.from({length:7},(_,i)=>`<div>${formatDate(addDays(start,i),{weekday:"short"})}</div>`).join("");
  const cells=Array.from({length:days},(_,i)=>{
    const day=addDays(start,i),events=eventsForDay(day).sort((a,b)=>new Date(a.starts_at)-new Date(b.starts_at));
    const visible=events.slice(0,state.viewDays===30?5:7);
    const more=events.length-visible.length;
    const monthMarker=i===0||day.getDate()===1?`<span class="month-marker">${formatDate(day,{month:"short"})}</span>`:"";
    return `<article class="day-cell ${sameDay(day,new Date())?"today":""} ${[0,6].includes(day.getDay())?"weekend":""}" data-date="${day.toISOString()}">
      <header><span>${monthMarker}</span><strong>${day.getDate()}</strong><button type="button" class="day-add" data-add-date="${escapeAttr(day.toISOString())}" aria-label="Add event on ${escapeAttr(formatDate(day,{month:"long",day:"numeric"}))}">+</button></header>
      <div class="day-events">${visible.map(e=>calendarEventBlock(e,false)).join("")}${more>0?`<span class="more-events">+${more} more</span>`:""}</div>
    </article>`;
  }).join("");
  return `<div class="day-grid-shell"><div class="day-grid-weekdays">${weekdayHeader}</div><div class="day-grid" style="--grid-days:7">${cells}</div></div>`;
}

function calendarEventBlock(e,compact=false){
  const time=e.all_day?"All day":formatTime(new Date(e.starts_at));
  const meta=[time,e.category_name].filter(Boolean).join(" · ");
  return `<button class="calendar-event ${compact?"compact":""}" data-event-key="${escapeAttr(eventKey(e))}" style="--cal:${safeColor(e.calendar_color||e.color)}">
    <span class="event-color"></span><span class="calendar-event-copy"><strong>${e.is_recurring?'<span class="repeat-mark" title="Repeating event">↻</span> ':""}${escapeHTML(e.title)}</strong><small>${escapeHTML(meta)}</small></span>${avatarMini(e)}
  </button>`;
}
function avatarMini(e){
  const people=e.assignees||[];
  if(!people.length)return "";
  return `<span class="mini-avatars">${people.slice(0,3).map(person=>`<i title="${escapeAttr(person.display_name)}">${person.avatar_url?`<img src="${escapeAttr(person.avatar_url)}" alt="">`:escapeHTML(person.initials)}</i>`).join("")}${people.length>3?`<i>+${people.length-3}</i>`:""}</span>`;
}
function bindCalendarEvents(){
  const host=$("#calendar-view");
  host.querySelectorAll("[data-event-key]").forEach(el=>el.addEventListener("click",event=>{
    event.stopPropagation();
    const found=findEventByKey(el.dataset.eventKey);if(found)requestEventEdit(found);
  }));
  host.querySelectorAll("[data-add-date]").forEach(button=>button.addEventListener("click",event=>{
    event.stopPropagation();
    const start=new Date(button.dataset.addDate);start.setHours(9,0,0,0);openEvent(null,start);
  }));
  host.querySelectorAll(".timed-lane").forEach(lane=>lane.addEventListener("click",event=>{
    if(event.target.closest("[data-event-key]")||event.target.closest(".current-time-line"))return;
    const rect=lane.getBoundingClientRect();
    const fraction=clamp((event.clientY-rect.top)/rect.height,0,0.999);
    const minutes=clamp(Math.round((fraction*24*60)/15)*15,0,23*60+45);
    const start=new Date(lane.dataset.day);start.setHours(0,minutes,0,0);
    openEvent(null,start);
  }));
}
function scrollCalendarNearNow(){
  if(!state.scrollNow||state.viewDays>7)return;
  const start=calendarViewStart(),end=addDays(start,state.viewDays);
  const now=new Date();
  if(now<start||now>=end)return;
  requestAnimationFrame(()=>{
    const host=$("#calendar-view");
    const minutes=now.getHours()*60+now.getMinutes();
    host.scrollTop=Math.max(0,minutes/(24*60)*1152-180);
  });
}

function renderAgenda(){
  const query=($("#agenda-search")?.value||"").trim().toLowerCase();
  const events=visibleEvents().filter(e=>{
    if(!query)return true;
    return [e.title,e.location,e.notes,e.calendar_name,...(e.assignees||[]).map(a=>a.display_name)].join(" ").toLowerCase().includes(query);
  }).sort((a,b)=>new Date(a.starts_at)-new Date(b.starts_at));
  $("#event-count").textContent=`${events.length} event${events.length===1?"":"s"}`;
  $("#events").innerHTML=events.length?events.map(eventCard).join(""):'<div class="empty-state"><strong>No matching events</strong><span>Add an event or change your filters.</span></div>';
  $("#events").querySelectorAll("[data-event-key]").forEach(el=>el.addEventListener("click",()=>{
    const ev=findEventByKey(el.dataset.eventKey);if(ev)requestEventEdit(ev);
  }));
}
function eventCard(e){
  const start=new Date(e.starts_at),end=new Date(e.ends_at);
  return `<button class="agenda-event" data-event-key="${escapeAttr(eventKey(e))}" style="--cal:${safeColor(e.calendar_color||e.color)}">
    <span class="agenda-color"></span><span class="agenda-date"><strong>${formatDate(start,{month:"short",day:"numeric"})}</strong><small>${e.all_day?"All day":formatTime(start)}</small></span>
    <span class="agenda-main"><strong>${e.is_recurring?'<span class="repeat-mark" title="Repeating event">↻</span> ':""}${escapeHTML(e.title)}</strong><small>${escapeHTML(e.calendar_name)}${e.location?" · "+escapeHTML(e.location):""}${e.all_day?"":` · ends ${escapeHTML(formatTime(end))}`}</small></span>
    ${avatarMini(e)}
  </button>`;
}

function reminderLabel(minutes){
  const n=Number(minutes)||0;
  if(n<60)return n+" min before";
  if(n%10080===0)return (n/10080)+" week"+(n===10080?"":"s")+" before";
  if(n%1440===0)return (n/1440)+" day"+(n===1440?"":"s")+" before";
  if(n%60===0)return (n/60)+" hour"+(n===60?"":"s")+" before";
  return n+" min before";
}

function notificationRows(){
  const now=Date.now();
  const rows=[];
  visibleEvents().forEach(event=>{
    const start=new Date(event.starts_at);
    (event.reminders||[]).forEach(reminder=>{
      const fireAt=new Date(start.getTime()-Number(reminder.minutes_before||0)*60000);
      if(fireAt.getTime()<now-60*60*1000)return;
      rows.push({event,reminder,fireAt});
    });
  });
  return rows.sort((a,b)=>a.fireAt-b.fireAt);
}

function renderNotifications(){
  const host=$("#scheduled-reminders"),summary=$("#notification-summary");
  if(!host||!summary)return;
  const filter=$("#notification-kind-filter")?.value||"";
  const all=notificationRows();
  const rows=filter?all.filter(row=>row.reminder.kind===filter):all;
  const personal=all.filter(row=>row.reminder.kind==="personal").length;
  const household=all.filter(row=>row.reminder.kind==="system").length;
  summary.innerHTML=`<article class="notification-stat"><span>Upcoming</span><strong>${all.length}</strong><small>scheduled reminders</small></article>
    <article class="notification-stat"><span>Personal</span><strong>${personal}</strong><small>phone reminders</small></article>
    <article class="notification-stat"><span>Household</span><strong>${household}</strong><small>Monita reminders</small></article>`;
  $("#notification-count").textContent=rows.length+" reminder"+(rows.length===1?"":"s");
  host.innerHTML=rows.length?rows.slice(0,200).map(({event,reminder,fireAt})=>{
    const system=reminder.kind==="system";
    const people=(event.assignees||[]).map(person=>person.display_name).join(", ");
    return `<button type="button" class="scheduled-reminder" data-event-key="${escapeAttr(eventKey(event))}">
      <span class="scheduled-reminder-icon ${system?"system":"personal"}">${system?"M":"P"}</span>
      <span class="scheduled-reminder-when"><strong>${escapeHTML(formatDate(fireAt,{weekday:"short",month:"short",day:"numeric"}))}</strong><small>${escapeHTML(formatTime(fireAt))}</small></span>
      <span class="scheduled-reminder-main"><strong>${escapeHTML(event.title)}</strong><small>${escapeHTML(reminderLabel(reminder.minutes_before))} · ${system?"Household via Monita":"Personal phone reminder"}${people?" · "+escapeHTML(people):""}</small></span>
      <span class="scheduled-reminder-calendar"><i style="--cal:${safeColor(event.calendar_color||event.color)}"></i>${escapeHTML(event.calendar_name||"Calendar")}</span>
    </button>`;
  }).join(""):'<div class="empty-state"><strong>No scheduled reminders</strong><span>Add a reminder to an event and it will appear here.</span></div>';
  host.querySelectorAll("[data-event-key]").forEach(button=>button.addEventListener("click",()=>{
    const event=findEventByKey(button.dataset.eventKey);
    if(event)requestEventEdit(event);
  }));
}

function renderPeople(){
  if(state.me.role!=="admin")return;
  $("#people-list").innerHTML=state.users.map(u=>`<article class="management-card ${u.active===false?"inactive":""}">
    <div class="management-avatar">${avatarMarkup(u)}</div>
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
  $("#person-avatar-file").value="";setAvatarPreview($("#person-avatar-preview"),null);
  $("#remove-person-avatar").classList.add("hidden");
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
  $("#person-avatar-file").value="";setAvatarPreview($("#person-avatar-preview"),user);
  $("#remove-person-avatar").classList.toggle("hidden",!user.avatar_url);
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
  $("#calendar-interop").classList.add("hidden");$("#calendar-interop-status").textContent="";$("#import-calendar-file").value="";
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
  $("#calendar-interop").classList.remove("hidden");$("#calendar-interop-status").textContent="";$("#import-calendar-file").value="";
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

function renderCategories(){
  if(state.me.role!=="admin")return;
  $("#category-list").innerHTML=state.categories.map(cat=>`<article class="management-card">
    <span class="calendar-swatch" style="--cal:${safeColor(cat.color)}"></span>
    <div class="management-copy"><strong>${escapeHTML(cat.name)}</strong><span>${escapeHTML(cat.description||"No description")}</span><small>Category marker ${escapeHTML(cat.color)}</small></div>
    <button class="button secondary compact edit-category" type="button" data-category-id="${cat.id}">Edit</button>
  </article>`).join("")||'<div class="empty-state"><strong>No categories</strong><span>Create a category to color-code event types.</span></div>';
  $("#category-list").querySelectorAll(".edit-category").forEach(b=>b.addEventListener("click",()=>beginCategoryEdit(b.dataset.categoryId)));
}

function resetCategoryForm(){
  const form=$("#category-form");if(!form)return;
  form.reset();form.color.value="#dc2626";form.category_id.value="";state.editingCategory=null;
  $("#category-form-eyebrow").textContent="New category";$("#category-form-title").textContent="Create a category";
  $("#delete-category").classList.add("hidden");$("#cancel-category-edit").classList.add("hidden");
  $("#category-error").textContent="";$("#category-status").textContent="";
}

function beginCategoryEdit(id){
  const cat=state.categories.find(item=>item.id===id);if(!cat)return;
  state.editingCategory=cat;const form=$("#category-form");
  form.category_id.value=cat.id;form.name.value=cat.name;form.color.value=cat.color;form.description.value=cat.description||"";
  $("#category-form-eyebrow").textContent="Edit category";$("#category-form-title").textContent=cat.name;
  $("#delete-category").classList.remove("hidden");$("#cancel-category-edit").classList.remove("hidden");
  $("#category-error").textContent="";$("#category-status").textContent="";
}

function activateSettingsTab(tab){
  if(tab==="general"&&state.me?.role!=="admin")tab="calendar";
  state.settingsTab=tab;
  $$$(".settings-nav-item").forEach(button=>button.classList.toggle("active",button.dataset.settingsTab===tab));
  $$("[data-settings-pane]").forEach(pane=>pane.classList.toggle("hidden",pane.dataset.settingsPane!==tab));
}
function renderSettings(){
  const form=$("#general-settings-form");
  if(form){
    form.household_name.value=state.settings?.household_name||"";
    form.timezone.value=state.settings?.timezone||Intl.DateTimeFormat().resolvedOptions().timeZone||"UTC";
    form.week_start.value=state.settings?.week_start||"sunday";
    form.default_view.value=String(state.settings?.default_view||7);
  }
  renderEventControls();
  const duration=$("#settings-default-duration"),scroll=$("#settings-scroll-now");
  if(duration)duration.value=String(state.defaultDuration);
  if(scroll)scroll.checked=state.scrollNow;
  const visibility=$("#settings-calendar-visibility");
  if(visibility){
    visibility.innerHTML=state.calendars.map(cal=>`<label class="settings-calendar-choice"><input type="checkbox" data-calendar-visibility="${cal.id}" ${state.hiddenCalendars.has(cal.id)?"":"checked"}><span class="dot" style="--cal:${safeColor(cal.color)}"></span><span>${escapeHTML(cal.name)}</span></label>`).join("");
    visibility.querySelectorAll("[data-calendar-visibility]").forEach(input=>input.addEventListener("change",()=>{
      const id=input.dataset.calendarVisibility;
      if(input.checked)state.hiddenCalendars.delete(id);else state.hiddenCalendars.add(id);
      localStorage.setItem("calden_hidden_calendars",JSON.stringify([...state.hiddenCalendars]));
      renderCalendar();renderAgenda();
    }));
  }
  activateSettingsTab(state.settingsTab);
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

function chooseRecurringScope(action){
  return new Promise(resolve=>{
    const dialog=$("#recurrence-scope-dialog");
    $("#scope-dialog-title").textContent=action==="delete"?"Delete repeating event":"Edit repeating event";
    $("#scope-dialog-copy").textContent=action==="delete"
      ?"Do you want to delete just this occurrence, or the entire series?"
      :"Do you want to change just this occurrence, or the entire series?";
    $("#scope-occurrence").textContent=action==="delete"?"Delete this occurrence":"This occurrence";
    $("#scope-series").textContent=action==="delete"?"Delete entire series":"Entire series";
    const finish=value=>{dialog.close();resolve(value)};
    $("#scope-cancel").onclick=()=>finish(null);
    $("#scope-occurrence").onclick=()=>finish("occurrence");
    $("#scope-series").onclick=()=>finish("series");
    dialog.oncancel=event=>{event.preventDefault();finish(null)};
    dialog.showModal();
  });
}

async function requestEventEdit(event){
  if(!event.is_recurring){openEvent(event,null,"series");return}
  const scope=await chooseRecurringScope("edit");
  if(scope)openEvent(event,null,scope);
}

function seriesValue(event,key,fallback){
  const seriesKey="series_"+key;
  return event[seriesKey]!==undefined&&event[seriesKey]!==null?event[seriesKey]:fallback;
}

function reminderMinutesOptions(selected){
  const standard=[
    [5,"5 minutes before"],[10,"10 minutes before"],[15,"15 minutes before"],[30,"30 minutes before"],
    [60,"1 hour before"],[120,"2 hours before"],[1440,"1 day before"],[2880,"2 days before"],
    [4320,"3 days before"],[10080,"1 week before"]
  ];
  if(selected&&!standard.some(([value])=>value===Number(selected)))standard.push([Number(selected),Number(selected)+" minutes before"]);
  return standard.sort((a,b)=>a[0]-b[0]).map(([value,label])=>`<option value="${value}" ${Number(selected)===value?"selected":""}>${label}</option>`).join("");
}

function addReminderRow(kind,reminder={}){
  const personal=kind==="personal";
  const host=personal?$("#personal-reminders-list"):$("#system-reminders-list");
  const minutes=Number(reminder.minutes_before||(personal?30:1440));
  const row=document.createElement("div");
  row.className=personal?"reminder-row":"reminder-row system-row";
  row.dataset.kind=kind;
  row.innerHTML=`<select class="reminder-minutes" aria-label="${personal?"Personal":"Household"} reminder time">${reminderMinutesOptions(minutes)}</select>
    ${personal?"":`<input class="reminder-destination" maxlength="200" placeholder="Monita channel (optional)" value="${escapeAttr(reminder.destination||"")}">`}
    <button type="button" class="icon-button reminder-remove" aria-label="Remove reminder">×</button>`;
  row.querySelector(".reminder-remove").addEventListener("click",()=>row.remove());
  host.appendChild(row);
}

function renderReminderEditor(reminders=[]){
  $("#personal-reminders-list").innerHTML="";
  $("#system-reminders-list").innerHTML="";
  reminders.filter(r=>r.kind==="personal"&&r.provider==="android").forEach(r=>addReminderRow("personal",r));
  reminders.filter(r=>r.kind==="system"&&r.provider==="monita").forEach(r=>addReminderRow("system",r));
}

function collectReminders(){
  const reminders=[];
  $("#personal-reminders-list").querySelectorAll(".reminder-row").forEach(row=>{
    reminders.push({kind:"personal",provider:"android",minutes_before:Number(row.querySelector(".reminder-minutes").value),destination:""});
  });
  $("#system-reminders-list").querySelectorAll(".reminder-row").forEach(row=>{
    reminders.push({kind:"system",provider:"monita",minutes_before:Number(row.querySelector(".reminder-minutes").value),destination:row.querySelector(".reminder-destination")?.value.trim()||""});
  });
  return reminders;
}

function openEvent(existing=null,dateHint=null,scope="series"){
  if(!state.calendars.some(c=>c.can_edit)){
    if(state.me.role==="admin"){navigate("calendars");return}
    alert("You do not have a calendar you can add events to yet.");return;
  }
  const form=$("#event-form");form.reset();state.editingEvent=existing;state.editingScope=scope;
  $("#event-dialog-title").textContent=existing?(scope==="occurrence"?"Edit occurrence":"Edit series"):"Add event";
  $("#delete-event").classList.toggle("hidden",!existing);
  $("#repeat-editor").classList.toggle("hidden",!!existing&&scope==="occurrence");
  const scopeNote=$("#series-scope-note");
  scopeNote.classList.toggle("hidden",!existing||!existing.is_recurring);
  if(existing&&existing.is_recurring){
    scopeNote.textContent=scope==="occurrence"
      ?"You are changing only this occurrence. The rest of the series will stay unchanged."
      :"You are changing the entire repeating series.";
  }
  if(existing){
    const occurrenceScope=scope==="occurrence";
    const sourceTitle=occurrenceScope?existing.title:seriesValue(existing,"title",existing.title);
    const sourceCalendar=occurrenceScope?existing.calendar_id:seriesValue(existing,"calendar_id",existing.calendar_id);
    const sourceCategory=occurrenceScope?existing.category_id:seriesValue(existing,"category_id",existing.category_id);
    const sourceLocation=occurrenceScope?existing.location:seriesValue(existing,"location",existing.location);
    const sourceNotes=occurrenceScope?existing.notes:seriesValue(existing,"notes",existing.notes);
    const sourceAllDay=occurrenceScope?existing.all_day:seriesValue(existing,"all_day",existing.all_day);
    const sourceAssignees=occurrenceScope?(existing.assignees||[]):(existing.series_assignees||existing.assignees||[]);
    const sourceReminders=occurrenceScope?(existing.reminders||[]):(existing.series_reminders||existing.reminders||[]);
    form.event_id.value=existing.id;form.title.value=sourceTitle||"";form.calendar_id.value=sourceCalendar||"";form.category_id.value=sourceCategory||"";
    form.starts_at.value=localInput(new Date(occurrenceScope?existing.starts_at:(existing.series_starts_at||existing.starts_at)));
    form.ends_at.value=localInput(new Date(occurrenceScope?existing.ends_at:(existing.series_ends_at||existing.ends_at)));
    form.all_day.checked=!!sourceAllDay;form.location.value=sourceLocation||"";form.notes.value=sourceNotes||"";
    const recurrence=occurrenceScope?null:(existing.recurrence||null);
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
    renderReminderEditor(sourceReminders);
    [...form.querySelectorAll('input[name="assignee"]')].forEach(i=>i.checked=sourceAssignees.some(a=>a.id===i.value));
  }else{
    $("#repeat-editor").classList.remove("hidden");$("#series-scope-note").classList.add("hidden");
    const start=dateHint?new Date(dateHint):new Date(Date.now()+3600000);
    if(!dateHint)start.setMinutes(0,0,0);else start.setSeconds(0,0);
    if(dateHint&&start.getHours()===0&&start.getMinutes()===0)start.setHours(9);
    const end=new Date(start.getTime()+state.defaultDuration*60000);
    form.starts_at.value=localInput(start);form.ends_at.value=localInput(end);
    const editable=state.calendars.filter(cal=>cal.can_edit);
    const preferred=editable.find(cal=>cal.id===state.defaultCalendar)||editable[0];
    if(preferred)form.calendar_id.value=preferred.id;
    form.repeat_frequency.value="";form.repeat_interval.value="1";form.repeat_end_type.value="never";form.repeat_count.value="10";
    [...form.querySelectorAll('input[name="repeat_weekday"]')].forEach(i=>i.checked=false);
    renderReminderEditor([]);
  }
  updateRepeatUI();
  $("#event-error").textContent="";$("#event-dialog").showModal();
}

function humanSize(bytes){
  const value=Number(bytes)||0;if(value<1024)return value+" B";
  const units=["KB","MB","GB","TB"];let n=value/1024,i=0;
  while(n>=1024&&i<units.length-1){n/=1024;i++}
  return n.toFixed(n>=10?1:2)+" "+units[i];
}
function downloadBlob(blob,filename){
  const url=URL.createObjectURL(blob);const a=document.createElement("a");
  a.href=url;a.download=filename||"calden-backup";document.body.appendChild(a);a.click();a.remove();
  setTimeout(()=>URL.revokeObjectURL(url),1500);
}
async function authenticatedDownload(path,fallbackName){
  const res=await fetch(path,{headers:{Authorization:"Bearer "+state.token}});
  if(!res.ok){
    const body=await res.json().catch(()=>null);throw new Error(body?.error||"Download failed");
  }
  const disposition=res.headers.get("Content-Disposition")||"";
  const match=disposition.match(/filename="?([^"]+)"?/i);
  const blob=await res.blob();downloadBlob(blob,match?.[1]||fallbackName);
}
async function loadBackups(){
  if(state.me.role!=="admin")return;
  try{
    const status=await api("/api/system/backup/status");
    renderBackupStatus(status);
  }catch(err){
    $("#restore-status").textContent=err.message;
    $("#saved-backups").innerHTML='<div class="empty-state"><strong>Could not load backups</strong></div>';
  }
}
function renderBackupStatus(status){
  $("#cancel-restore").classList.toggle("hidden",!status.pending);
  $("#restart-for-restore").classList.toggle("hidden",!status.pending);
  if(status.pending){
    $("#restore-status").textContent="A validated restore is staged and ready. Restart CalDen to apply it.";
  }else if(!$("#restore-status").textContent.includes("staged")){
    $("#restore-status").textContent="No restore is currently staged.";
  }
  const last=status.last||null,box=$("#restore-last-result");
  box.classList.toggle("hidden",!last);
  if(last){
    const ok=last.status==="success";
    box.className="restore-result "+(ok?"restore-success":"restore-failed");
    box.innerHTML=`<strong>${ok?"Last restore completed":"Last restore did not complete"}</strong><span>Status: ${escapeHTML(last.status||"unknown")}${last.safety_backup?" · Safety backup: "+escapeHTML(last.safety_backup):""}</span>`;
  }
  const backups=status.backups||[];
  $("#saved-backups").innerHTML=backups.length?backups.map(b=>`<article class="saved-backup">
    <div><strong>${escapeHTML(b.name)}</strong><span>${humanSize(b.size)} · ${escapeHTML(formatDate(new Date(b.created_at),{month:"short",day:"numeric",year:"numeric"}))} at ${escapeHTML(formatTime(new Date(b.created_at)))}</span></div>
    <div class="saved-backup-actions"><button class="button secondary compact backup-download" data-name="${escapeAttr(b.name)}" type="button">Download</button><button class="text-button backup-delete" data-name="${escapeAttr(b.name)}" type="button">Delete</button></div>
  </article>`).join(""):'<div class="empty-state"><strong>No saved backups yet</strong><span>Create a backup or install an update to create safety copies.</span></div>';
  $("#saved-backups").querySelectorAll(".backup-download").forEach(b=>b.addEventListener("click",async()=>{
    try{await authenticatedDownload("/api/system/backups/"+encodeURIComponent(b.dataset.name),b.dataset.name)}
    catch(err){$("#restore-status").textContent=err.message}
  }));
  $("#saved-backups").querySelectorAll(".backup-delete").forEach(b=>b.addEventListener("click",async()=>{
    if(!confirm(`Delete saved backup "${b.dataset.name}"? This cannot be undone.`))return;
    try{await api("/api/system/backups/"+encodeURIComponent(b.dataset.name),{method:"DELETE"});await loadBackups()}
    catch(err){$("#restore-status").textContent=err.message}
  }));
}

function activityVerb(item){
  const labels={
    create:"Created",update:"Updated",delete:"Deleted",archive:"Archived",deactivate:"Deactivated",
    permissions:"Changed access for",update_occurrence:"Changed one occurrence of",
    delete_occurrence:"Deleted one occurrence of",restore_occurrence:"Restored one occurrence of",
    preferences:"Changed preferences for",install:"Started update",rollback:"Started rollback",test:"Tested"
  };
  return labels[item.action]||item.action.replaceAll("_"," ");
}
function activityEntityLabel(type){
  return ({event:"event",calendar:"calendar",category:"category",user:"person",settings:"household settings",integration:"integration",update:"CalDen"})[type]||type;
}
async function loadActivity(){
  if(state.me.role!=="admin")return;
  const host=$("#activity-list");host.innerHTML='<div class="empty-state"><strong>Loading activity…</strong></div>';
  const filter=$("#activity-filter").value;
  try{
    const items=await api("/api/activity?limit=250"+(filter?"&entity_type="+encodeURIComponent(filter):""));
    host.innerHTML=items.length?items.map(activityRow).join(""):'<div class="empty-state"><strong>No activity yet</strong><span>Changes made in CalDen will appear here.</span></div>';
  }catch(err){
    host.innerHTML=`<div class="empty-state"><strong>Could not load activity</strong><span>${escapeHTML(err.message)}</span></div>`;
  }
}
function activityRow(item){
  const actor=item.actor||{};
  const date=new Date(item.created_at);
  const actorName=actor.display_name||"System";
  const initials=actor.initials||"•";
  const summary=item.summary||`${activityVerb(item)} ${activityEntityLabel(item.entity_type)}`;
  return `<article class="activity-entry">
    <div class="activity-avatar">${escapeHTML(initials)}</div>
    <div class="activity-copy">
      <div><strong>${escapeHTML(actorName)}</strong> <span>${escapeHTML(summary)}</span></div>
      <small>${escapeHTML(formatDate(date,{weekday:"short",month:"short",day:"numeric",year:"numeric"}))} at ${escapeHTML(formatTime(date))} · ${escapeHTML(item.entity_type)}</small>
    </div>
  </article>`;
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
$("#calendar-strip").addEventListener("click",e=>{
  const b=e.target.closest("[data-calendar-id]");if(!b)return;
  const id=b.dataset.calendarId;if(state.hiddenCalendars.has(id))state.hiddenCalendars.delete(id);else state.hiddenCalendars.add(id);
  localStorage.setItem("calden_hidden_calendars",JSON.stringify([...state.hiddenCalendars]));renderCalendar();renderAgenda();renderSettings();
});
$$(".view-switcher button").forEach(b=>b.addEventListener("click",async()=>{
  state.viewDays=Number(b.dataset.days);localStorage.setItem("calden_view_days",String(state.viewDays));
  await loadEvents();renderCalendar();renderAgenda();
}));
$("#calendar-prev").addEventListener("click",async()=>{state.anchorDate=addDays(state.anchorDate,-state.viewDays);await loadEvents();renderCalendar();renderAgenda()});
$("#calendar-next").addEventListener("click",async()=>{state.anchorDate=addDays(state.anchorDate,state.viewDays);await loadEvents();renderCalendar();renderAgenda()});
$("#calendar-today").addEventListener("click",async()=>{state.anchorDate=startOfDay(new Date());await loadEvents();renderCalendar();renderAgenda()});
$("#calendar-category-filter").addEventListener("change",e=>{state.filters.category=e.target.value;renderCalendar();renderAgenda()});
$("#calendar-person-filter").addEventListener("change",e=>{state.filters.person=e.target.value;renderCalendar();renderAgenda()});
$("#calendar-search").addEventListener("input",e=>{state.filters.query=e.target.value.trim();renderCalendar();renderAgenda()});
$("#clear-calendar-filters").addEventListener("click",()=>{
  state.filters={category:"",person:"",query:""};renderCalendar();renderAgenda();
});
$("#agenda-search").addEventListener("input",renderAgenda);
$("#notification-kind-filter").addEventListener("change",renderNotifications);
$("#refresh-notifications").addEventListener("click",async()=>{await loadEvents();renderNotifications()});
$$("#event-dialog [data-close-event]").forEach(b=>b.addEventListener("click",()=>$("#event-dialog").close()));
$("#add-personal-reminder").addEventListener("click",()=>addReminderRow("personal"));
$("#add-system-reminder").addEventListener("click",()=>addReminderRow("system"));
$("#event-form").repeat_frequency.addEventListener("change",updateRepeatUI);
$("#event-form").repeat_interval.addEventListener("input",updateRepeatUI);
$("#event-form").repeat_end_type.addEventListener("change",updateRepeatUI);
$("#event-form").starts_at.addEventListener("change",()=>{if($("#event-form").repeat_frequency.value==="weekly")updateRepeatUI()});
$("#delete-event").addEventListener("click",async()=>{
  if(!state.editingEvent)return;
  let scope=state.editingScope;
  if(state.editingEvent.is_recurring&&scope!=="occurrence"){
    $("#event-dialog").close();
    scope=await chooseRecurringScope("delete");
    if(!scope){$("#event-dialog").showModal();return}
  }else if(!confirm("Delete this event?"))return;
  try{
    if(state.editingEvent.is_recurring&&scope==="occurrence"){
      await api("/api/events/"+state.editingEvent.id+"/occurrences",{
        method:"DELETE",
        body:JSON.stringify({original_start:state.editingEvent.occurrence_start||state.editingEvent.starts_at})
      });
    }else{
      await api("/api/events/"+state.editingEvent.id,{method:"DELETE"});
    }
    if($("#event-dialog").open)$("#event-dialog").close();
    state.editingEvent=null;state.editingScope="series";
    await loadEvents();renderCalendar();renderAgenda();
  }catch(err){
    if(!$("#event-dialog").open)$("#event-dialog").showModal();
    $("#event-error").textContent=err.message;
  }
});
$("#event-form").addEventListener("submit",async e=>{
  e.preventDefault();const form=e.currentTarget,fd=new FormData(form);$("#event-error").textContent="";
  const reminders=collectReminders();
  const payload={title:fd.get("title"),calendar_id:fd.get("calendar_id"),category_id:fd.get("category_id")||null,starts_at:new Date(fd.get("starts_at")).toISOString(),ends_at:new Date(fd.get("ends_at")).toISOString(),all_day:form.all_day.checked,location:fd.get("location"),notes:fd.get("notes"),assignee_ids:fd.getAll("assignee"),reminders,recurrence:recurrencePayload(form)};
  let target="/api/events",method="POST",body=payload;
  if(state.editingEvent){
    if(state.editingEvent.is_recurring&&state.editingScope==="occurrence"){
      payload.recurrence=null;
      target="/api/events/"+state.editingEvent.id+"/occurrences";method="PUT";
      body={original_start:state.editingEvent.occurrence_start||state.editingEvent.starts_at,event:payload};
    }else{
      target="/api/events/"+state.editingEvent.id;method="PUT";
    }
  }
  try{
    await api(target,{method,body:JSON.stringify(body)});
    $("#event-dialog").close();state.editingEvent=null;state.editingScope="series";
    await loadEvents();renderCalendar();renderAgenda();
  }
  catch(err){$("#event-error").textContent=err.message}
});

$("#person-form").addEventListener("submit",async e=>{
  e.preventDefault();const form=e.currentTarget;$("#person-error").textContent="";$("#person-status").textContent="";
  const editing=state.editingUser;
  try{
    let userID=editing?.id||"";
    if(editing){
      const payload={display_name:form.display_name.value.trim(),role:form.role.value,active:form.active.checked,password:form.password.value};
      await api("/api/users/"+editing.id,{method:"PUT",body:JSON.stringify(payload)});
    }else{
      const payload={display_name:form.display_name.value.trim(),username:form.username.value.trim(),role:form.role.value,password:form.password.value};
      const created=await api("/api/users",{method:"POST",body:JSON.stringify(payload)});
      userID=created.id;
    }
    const avatarFile=$("#person-avatar-file").files?.[0];
    if(avatarFile&&userID){
      const data=new FormData();data.append("avatar",avatarFile);
      await apiForm("/api/users/"+userID+"/avatar",data);
    }
    state.users=await api("/api/users");
    resetPersonForm();renderPeople();renderCalendarPermissionChecks();renderEventControls();
    $("#person-status").textContent=editing?"Person updated.":"Person added.";
  }catch(err){$("#person-error").textContent=err.message}
});
$("#cancel-person-edit").addEventListener("click",resetPersonForm);
$("#person-avatar-file").addEventListener("change",e=>{
  const file=e.target.files?.[0];if(!file)return;
  const url=URL.createObjectURL(file);
  $("#person-avatar-preview").innerHTML=`<img src="${url}" alt="">`;
});
$("#remove-person-avatar").addEventListener("click",async()=>{
  if(!state.editingUser)return;
  try{
    await api("/api/users/"+state.editingUser.id+"/avatar",{method:"DELETE"});
    state.users=await api("/api/users");
    const refreshed=state.users.find(u=>u.id===state.editingUser.id);
    if(refreshed)state.editingUser=refreshed;
    setAvatarPreview($("#person-avatar-preview"),state.editingUser);
    $("#remove-person-avatar").classList.add("hidden");
    renderPeople();renderEventControls();
    $("#person-status").textContent="Profile image removed.";
  }catch(err){$("#person-error").textContent=err.message}
});

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
$("#export-calendar-ics").addEventListener("click",async()=>{
  if(!state.editingCalendar)return;
  $("#calendar-interop-status").textContent="Preparing iCalendar export…";
  try{
    await authenticatedDownload("/api/calendars/"+state.editingCalendar.id+"/export.ics",state.editingCalendar.name+".ics");
    $("#calendar-interop-status").textContent="Calendar exported.";
  }catch(err){$("#calendar-interop-status").textContent=err.message}
});
$("#import-calendar-ics").addEventListener("click",async()=>{
  if(!state.editingCalendar)return;
  const file=$("#import-calendar-file").files?.[0];
  if(!file){$("#calendar-interop-status").textContent="Choose an .ics file first.";return}
  const button=$("#import-calendar-ics");button.disabled=true;button.textContent="Importing…";$("#calendar-interop-status").textContent="Reading iCalendar events…";
  try{
    const form=new FormData();form.append("calendar",file,file.name);
    const res=await fetch("/api/calendars/"+state.editingCalendar.id+"/import.ics",{
      method:"POST",headers:{Authorization:"Bearer "+state.token},body:form
    });
    const body=await res.json().catch(()=>null);if(!res.ok)throw new Error(body?.error||"iCalendar import failed");
    $("#calendar-interop-status").textContent=`Imported: ${body.created||0} new, ${body.updated||0} updated, ${body.exceptions||0} recurrence changes${body.skipped?" · "+body.skipped+" skipped":""}.`;
    [state.categories,state.calendars]=await Promise.all([api("/api/categories"),api("/api/calendars")]);
    await loadEvents();renderCategories();renderCalendars();renderEventControls();renderCalendar();renderAgenda();
  }catch(err){$("#calendar-interop-status").textContent=err.message}
  finally{button.disabled=false;button.textContent="Import .ics"}
});

$("#delete-calendar").addEventListener("click",async()=>{
  if(!state.editingCalendar||!confirm(`Delete "${state.editingCalendar.name}"? The calendar must be empty first.`))return;
  try{await api("/api/calendars/"+state.editingCalendar.id,{method:"DELETE"});state.calendars=await api("/api/calendars");resetCalendarForm();renderCalendars();renderCalendar();renderEventControls()}
  catch(err){$("#calendar-error").textContent=err.message}
});

$("#category-form").addEventListener("submit",async e=>{
  e.preventDefault();const form=e.currentTarget;$("#category-error").textContent="";$("#category-status").textContent="";
  const payload={name:form.name.value.trim(),color:form.color.value,icon:"tag",description:form.description.value.trim()};
  try{
    if(state.editingCategory){
      await api("/api/categories/"+state.editingCategory.id,{method:"PUT",body:JSON.stringify(payload)});
      $("#category-status").textContent="Category updated.";
    }else{
      await api("/api/categories",{method:"POST",body:JSON.stringify(payload)});
      $("#category-status").textContent="Category created.";
    }
    state.categories=await api("/api/categories");resetCategoryForm();renderCategories();renderEventControls();await loadEvents();renderCalendar();renderAgenda();
  }catch(err){$("#category-error").textContent=err.message}
});
$("#cancel-category-edit").addEventListener("click",resetCategoryForm);
$("#delete-category").addEventListener("click",async()=>{
  if(!state.editingCategory||!confirm(`Archive "${state.editingCategory.name}"? Existing events keep the category label; their calendar still controls the event color.`))return;
  try{
    await api("/api/categories/"+state.editingCategory.id,{method:"DELETE"});
    state.categories=await api("/api/categories");resetCategoryForm();renderCategories();renderEventControls();await loadEvents();renderCalendar();renderAgenda();
  }catch(err){$("#category-error").textContent=err.message}
});

$$(".settings-nav-item").forEach(button=>button.addEventListener("click",()=>activateSettingsTab(button.dataset.settingsTab)));
$("#display-settings-form").addEventListener("submit",e=>{
  e.preventDefault();
  state.defaultCalendar=$("#settings-default-calendar").value;
  state.defaultDuration=Number($("#settings-default-duration").value)||60;
  state.scrollNow=$("#settings-scroll-now").checked;
  localStorage.setItem("calden_default_calendar",state.defaultCalendar);
  localStorage.setItem("calden_default_duration",String(state.defaultDuration));
  localStorage.setItem("calden_scroll_now",String(state.scrollNow));
  $("#display-settings-status").textContent="Display settings saved.";
  renderCalendar();
});
$("#reset-display-settings").addEventListener("click",()=>{
  state.defaultCalendar="";state.defaultDuration=60;state.scrollNow=true;
  localStorage.removeItem("calden_default_calendar");
  localStorage.removeItem("calden_default_duration");
  localStorage.removeItem("calden_scroll_now");
  state.hiddenCalendars.clear();
  localStorage.setItem("calden_hidden_calendars","[]");
  $("#display-settings-status").textContent="Display settings reset.";
  renderSettings();renderCalendar();renderAgenda();
});

$("#general-settings-form").addEventListener("submit",async e=>{
  e.preventDefault();const form=e.currentTarget;if(state.me.role!=="admin")return;
  $("#general-settings-status").textContent="";
  const payload={household_name:form.household_name.value.trim(),timezone:form.timezone.value.trim(),week_start:form.week_start.value,default_view:Number(form.default_view.value)};
  try{
    state.settings=await api("/api/settings/general",{method:"PUT",body:JSON.stringify(payload)});
    $("#household-label").textContent=state.settings.household_name;
    $("#general-settings-status").textContent="Household settings saved.";
    renderCalendar();
  }catch(err){$("#general-settings-status").textContent=err.message}
});


$("#settings-avatar-file").addEventListener("change",e=>{
  const file=e.target.files?.[0];if(!file)return;
  $("#settings-avatar-preview").innerHTML=`<img src="${URL.createObjectURL(file)}" alt="">`;
});
$("#save-settings-avatar").addEventListener("click",async()=>{
  const file=$("#settings-avatar-file").files?.[0];
  if(!file){$("#settings-avatar-status").textContent="Choose a profile image first.";return}
  $("#settings-avatar-status").textContent="Uploading…";
  try{
    const data=new FormData();data.append("avatar",file);
    await apiForm("/api/users/"+state.me.id+"/avatar",data);
    state.me=await api("/api/me");
    $("#settings-avatar-file").value="";
    setAvatarPreview($("#settings-avatar-preview"),state.me);
    $("#sidebar-avatar").innerHTML=state.me.avatar_url?`<img src="${escapeAttr(state.me.avatar_url)}" alt="">`:escapeHTML(state.me.initials||"?");
    $("#remove-settings-avatar").classList.toggle("hidden",!state.me.avatar_url);
    $("#settings-avatar-status").textContent="Profile image updated.";
  }catch(err){$("#settings-avatar-status").textContent=err.message}
});
$("#remove-settings-avatar").addEventListener("click",async()=>{
  $("#settings-avatar-status").textContent="";
  try{
    await api("/api/users/"+state.me.id+"/avatar",{method:"DELETE"});
    state.me=await api("/api/me");
    setAvatarPreview($("#settings-avatar-preview"),state.me);
    $("#sidebar-avatar").innerHTML=escapeHTML(state.me.initials||"?");
    $("#remove-settings-avatar").classList.add("hidden");
    $("#settings-avatar-status").textContent="Profile image removed.";
  }catch(err){$("#settings-avatar-status").textContent=err.message}
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

$("#create-backup").addEventListener("click",async()=>{
  const button=$("#create-backup");button.disabled=true;button.textContent="Creating backup…";$("#restore-status").textContent="Creating PostgreSQL backup…";
  try{await authenticatedDownload("/api/system/backup/download","calden-backup.tar.gz");$("#restore-status").textContent="Backup created and downloaded.";await loadBackups()}
  catch(err){$("#restore-status").textContent=err.message}
  finally{button.disabled=false;button.textContent="Create & download backup"}
});
$("#refresh-backups").addEventListener("click",loadBackups);
$("#stage-restore").addEventListener("click",async()=>{
  const input=$("#restore-file"),file=input.files?.[0];if(!file){$("#restore-status").textContent="Choose a CalDen backup file first.";return}
  if(!confirm("Validate and stage this backup for restore? Nothing will be changed until CalDen restarts."))return;
  const button=$("#stage-restore");button.disabled=true;button.textContent="Validating…";$("#restore-status").textContent="Uploading and validating backup…";
  try{
    const form=new FormData();form.append("backup",file,file.name);
    const res=await fetch("/api/system/backup/restore",{method:"POST",headers:{Authorization:"Bearer "+state.token},body:form});
    const body=await res.json().catch(()=>null);if(!res.ok)throw new Error(body?.error||"Restore could not be staged");
    $("#restore-status").textContent=body.message||"Restore staged.";await loadBackups();
  }catch(err){$("#restore-status").textContent=err.message}
  finally{button.disabled=false;button.textContent="Validate & stage restore"}
});
$("#cancel-restore").addEventListener("click",async()=>{
  try{await api("/api/system/backup/restore",{method:"DELETE"});$("#restore-status").textContent="Staged restore cancelled.";$("#restore-file").value="";await loadBackups()}
  catch(err){$("#restore-status").textContent=err.message}
});
$("#restart-for-restore").addEventListener("click",async()=>{
  if(!confirm("Restart CalDen now and apply the staged restore? A pre-restore safety backup will be created automatically."))return;
  $("#restore-status").textContent="Restarting CalDen to apply restore…";
  try{
    await api("/api/system/restart",{method:"POST",body:"{}"});
    let attempts=0;
    const wait=async()=>{
      attempts++;
      try{
        const res=await fetch("/api/health",{cache:"no-store"});
        if(res.ok){location.reload();return}
      }catch{}
      if(attempts<90)setTimeout(wait,1500);else $("#restore-status").textContent="CalDen has not returned yet. Check container logs.";
    };
    setTimeout(wait,1800);
  }catch(err){$("#restore-status").textContent=err.message}
});

$("#activity-filter").addEventListener("change",loadActivity);
$("#refresh-activity").addEventListener("click",loadActivity);
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
resetCategoryForm();
boot().catch(err=>{console.error(err);showBootFailure()});
