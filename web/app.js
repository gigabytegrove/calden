let caldenClientReports=0;
function reportCalDenClientError(reason){
  if(caldenClientReports>=5)return;
  caldenClientReports++;
  const token=localStorage.getItem("calden_token");
  if(!token)return;
  fetch("/api/client-errors",{method:"POST",cache:"no-store",
    headers:{"Content-Type":"application/json","Authorization":"Bearer "+token},
    body:JSON.stringify({message:String(reason?.message||reason||"Unknown JavaScript error").slice(0,400),version:document.querySelector('meta[name="calden-version"]')?.content||"unknown"})
  }).catch(()=>{});
}
let caldenBooting=true;
window.addEventListener("error",event=>{console.error("Calden runtime error:",event.error||event.message);reportCalDenClientError(event.error||event.message);if(caldenBooting)showBootFailure(event.error||event.message)});
window.addEventListener("unhandledrejection",event=>{console.error("Calden unhandled promise rejection:",event.reason);reportCalDenClientError(event.reason);if(caldenBooting)showBootFailure(event.reason)});


const $=s=>document.querySelector(s);
const $$=s=>document.querySelectorAll(s);
const savedDays=Number(localStorage.getItem("calden_view_days")||0);
function readStoredJSON(key,fallback){
  try{return JSON.parse(localStorage.getItem(key)||JSON.stringify(fallback))}
  catch{return fallback}
}
const savedHidden=readStoredJSON("calden_hidden_calendars",[]);
const savedDefaultCalendar=localStorage.getItem("calden_default_calendar")||"";
const savedDefaultDuration=Number(localStorage.getItem("calden_default_duration")||60);
const savedScrollNow=localStorage.getItem("calden_scroll_now")!=="false";

const state={
  token:localStorage.getItem("calden_token")||"",
  me:null,settings:null,users:[],calendars:[],categories:[],events:[],chores:[],householdDevices:[],
  dashboard:null,shopping:{lists:[],items:[],listID:null},routines:[],rewards:null,routineDate:dateInputValue(new Date()),
  editingEvent:null,editingScope:"series",preserveRawRecurrence:false,editingCalendar:null,editingCategory:null,editingUser:null,
  setupStep:0,currentPage:"calendar",
  viewDays:[1,7,14,30].includes(savedDays)?savedDays:7,
  anchorDate:startOfDay(new Date()),
  hiddenCalendars:new Set(Array.isArray(savedHidden)?savedHidden:[]),
  filters:{category:"",person:"",query:""},
  defaultCalendar:savedDefaultCalendar,
  defaultDuration:[30,60,90,120].includes(savedDefaultDuration)?savedDefaultDuration:60,
  scrollNow:savedScrollNow,
  settingsTab:"general",
  billMonth:new Date(new Date().getFullYear(),new Date().getMonth(),1),
  billEvents:[],billPaymentEvent:null,billPaymentEditingId:null,
  notifications:[],unreadNotifications:0,notificationKnown:new Set(),notificationPoll:null,
  googleImportFile:null,googleImportPreview:null,googleRepairFile:null,googleRepairPreview:null,
  updateInfo:null,updatePoll:null,updatePrefsDirty:false,detailEvent:null,suppressDetailCloseRoute:false
};

async function api(path,options={}){
  const headers={"Content-Type":"application/json",...(options.headers||{})};
  if(state.token)headers.Authorization=`Bearer ${state.token}`;
  const res=await fetch(path,{cache:"no-store",...options,headers});
  const body=res.status===204?null:await res.json().catch(()=>null);
  if(!res.ok){
    const err=new Error(body?.error||`Request failed (${res.status})`);
    err.status=res.status;
    err.body=body;
    throw err;
  }
  return body;
}
async function apiForm(path,formData,method="POST"){
  const headers={};
  if(state.token)headers.Authorization=`Bearer ${state.token}`;
  const res=await fetch(path,{method,headers,body:formData,cache:"no-store"});
  const body=res.status===204?null:await res.json().catch(()=>null);
  if(!res.ok)throw new Error(body?.error||`Request failed (${res.status})`);
  return body;
}
function setToken(token){
  caldenNotificationSocket?.close();
  caldenNotificationSocket=null;
  clearTimeout(caldenNotificationRetry);
  state.token=token||"";
  if(token)localStorage.setItem("calden_token",token);else localStorage.removeItem("calden_token");
}
function formJSON(form){return Object.fromEntries(new FormData(form).entries())}
function escapeHTML(v=""){return String(v).replace(/[&<>"']/g,c=>({"&":"&amp;","<":"&lt;",">":"&gt;",'"':"&quot;","'":"&#39;"}[c]))}
function escapeAttr(v=""){return escapeHTML(v)}
function safeColor(v){return /^#[0-9a-f]{6}$/i.test(v)?v:"#667085"}
function startOfDay(value){const d=new Date(value);d.setHours(0,0,0,0);return d}
function addDays(value,days){const d=new Date(value);d.setDate(d.getDate()+days);return d}
function sameDay(a,b){return startOfDay(a).getTime()===startOfDay(b).getTime()}
function localInput(d){const p=n=>String(n).padStart(2,"0");return `${d.getFullYear()}-${p(d.getMonth()+1)}-${p(d.getDate())}T${p(d.getHours())}:${p(d.getMinutes())}`}
const maxAvatarUploadBytes=10*1024*1024;
function roleLabel(role){return role==="admin"?"Administrator":role==="restricted"?"Restricted member":role==="planner"?"Planner":"Family member"}
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
function previewAvatarFile(input,preview,status){
  const file=input?.files?.[0];
  if(!file)return true;
  if(file.size>maxAvatarUploadBytes){
    input.value="";
    if(status)status.textContent="Profile image must be 10 MB or smaller.";
    return false;
  }
  const allowed=["image/jpeg","image/png","image/webp"];
  if(file.type&&!allowed.includes(file.type)){
    input.value="";
    if(status)status.textContent="Use a JPG, PNG, or WebP profile image.";
    return false;
  }
  const url=URL.createObjectURL(file);
  const img=document.createElement("img");
  img.alt="";
  img.onload=()=>URL.revokeObjectURL(url);
  img.onerror=()=>{
    URL.revokeObjectURL(url);
    if(status)status.textContent="Could not preview this image. Use a JPG, PNG, or WebP file.";
    preview.innerHTML="<span>?</span>";
  };
  img.src=url;
  preview.replaceChildren(img);
  if(status)status.textContent="";
  return true;
}
function formatDate(d,opts={month:"short",day:"numeric"}){return new Intl.DateTimeFormat(undefined,opts).format(d)}
function formatTime(d){return new Intl.DateTimeFormat(undefined,{hour:"numeric",minute:"2-digit"}).format(d)}
function money(value){return new Intl.NumberFormat(undefined,{style:"currency",currency:"USD"}).format(Number(value)||0)}
function billCalendar(id){return state.calendars.find(cal=>cal.id===id&&cal.calendar_type==="bill_pay")}
function isBillCalendar(id){return !!billCalendar(id)}
function billAmountLabel(event){
  if(event?.bill_amount===null||event?.bill_amount===undefined)return "";
  return `${event.bill_amount_is_estimate?"~":""}${money(event.bill_amount)}`;
}
function billOccurrenceStart(event){return event?.occurrence_start||event?.starts_at}
function billPaymentStatus(event){return event?.bill_payment_status||(event?.bill_paid?"paid":"due")}
function billPayments(event){return Array.isArray(event?.bill_payments)?event.bill_payments:[]}
function billPaidAmount(event){
  if(event?.bill_amount_paid!==null&&event?.bill_amount_paid!==undefined)return Number(event.bill_amount_paid)||0;
  return billPayments(event).reduce((sum,payment)=>sum+(Number(payment.amount_paid)||0),0);
}
function billAllocatedAmount(event){
  if(event?.bill_amount_allocated===null||event?.bill_amount_allocated===undefined)return 0;
  return Number(event.bill_amount_allocated)||0;
}
function billStatusLabel(status){
  return status==="no_balance"?"No balance"
    :status==="allocated"?"Allocated"
    :status==="partial"?"Partial"
    :status==="paid"?"Paid"
    :status==="cleared"?"Cleared"
    :"Due";
}
function canUpdateBill(event){return !!state.calendars.find(cal=>cal.id===event?.calendar_id&&cal.calendar_type==="bill_pay"&&cal.can_edit)}
function billPaymentLabel(event){
  const status=billPaymentStatus(event);
  if(status==="no_balance")return "No balance";
  if(status==="allocated")return `Allocated ${money(billAllocatedAmount(event))}`;
  if(status==="partial")return `Partially paid ${money(billPaidAmount(event))}`;
  if(status==="cleared")return "Cleared";
  if(status!=="paid")return "";
  const who=event.bill_paid_by?.display_name;
  return who?`Paid by ${who}`:"Paid";
}
function billCalendarStatusMarker(event){
  if(!isBillCalendar(event?.calendar_id))return "";
  const status=billPaymentStatus(event);
  const marker=status==="allocated"
    ?{icon:"⌛︎",label:"Funds allocated"}
    :status==="paid"
      ?{icon:"✓",label:"Paid"}
      :status==="cleared"
        ?{icon:"✓",label:"Cleared"}
        :null;
  if(!marker)return "";
  return `<span class="bill-calendar-status ${status}" title="${escapeAttr(marker.label)}" aria-label="${escapeAttr(marker.label)}">${marker.icon}</span>`;
}
function dateInputValue(value=new Date()){
  // Date-only values are civil dates, not UTC instants. Parsing YYYY-MM-DD
  // with new Date() shifts the selected day backward west of UTC.
  const d=typeof value==="string"&&/^\d{4}-\d{2}-\d{2}$/.test(value)
    ? calendarDate(value)
    : new Date(value);
  if(Number.isNaN(d.getTime()))return "";
  const p=n=>String(n).padStart(2,"0");
  return `${d.getFullYear()}-${p(d.getMonth()+1)}-${p(d.getDate())}`;
}
function calendarDate(value){
  const match=String(value||"").match(/^(\d{4})-(\d{2})-(\d{2})/);
  if(!match)return new Date(value);
  return new Date(Number(match[1]),Number(match[2])-1,Number(match[3]));
}
function eventStartFromHint(dateHint=null){
  const start=dateHint
    ?(typeof dateHint==="string"&&/^\d{4}-\d{2}-\d{2}$/.test(dateHint)
      ?calendarDate(dateHint):new Date(dateHint))
    :new Date(Date.now()+3600000);
  if(!Number.isFinite(start.getTime()))throw new Error("Invalid selected appointment date");
  if(!dateHint)start.setMinutes(0,0,0);else start.setSeconds(0,0);
  if(dateHint&&start.getHours()===0&&start.getMinutes()===0)start.setHours(9);
  return start;
}
function eventStartDate(event){return event?.all_day?calendarDate(event.starts_at):new Date(event?.starts_at)}
function eventEndDate(event){return event?.all_day?calendarDate(event.ends_at):new Date(event?.ends_at)}
function clamp(n,min,max){return Math.min(max,Math.max(min,n))}
function eventKey(e){return `${e.id}|${e.occurrence_start||e.starts_at}`}
function findEventByKey(key){return state.events.find(e=>eventKey(e)===key)}

function showBootFailure(reason){
  const el=$("#boot-status");if(!el)return;
  el.classList.remove("hidden");el.classList.add("boot-error");
  const explanation=reason?.message||String(reason||"Unknown startup error");
  el.textContent="Calden could not finish loading: "+explanation+". Try Reload. If this continues, check the Calden container logs.";
  const retry=document.createElement("button");
  retry.type="button";retry.className="button secondary";
  retry.textContent="Reload Calden";
  retry.addEventListener("click",()=>location.reload());
  el.append(" ",retry);
}
function hideBootStatus(){const el=$("#boot-status");if(el)el.classList.add("hidden");caldenBooting=false}

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
  caldenBooting=true;
  // Restore the application shell immediately for an existing session.
  // Never show the full-page sign-in splash just to refresh calendar data.
  if(state.token){
    $("#auth").classList.add("hidden");
    $("#app").classList.remove("hidden");
  }
  const initial=readCalDenRoute();
  if(initial.date)state.anchorDate=startOfDay(initial.date);
  if(initial.month)state.billMonth=initial.month;
  if(initial.tab)state.settingsTab=initial.tab;
  if([1,7,14,30].includes(initial.days))state.viewDays=initial.days;
  const setup=await api("/api/setup/status");
  if(setup.needs_setup){showAuth("setup");return}
  if(!state.token){showAuth("login");return}
  try{
    state.me=await api("/api/me");
    state.settings=await api("/api/settings/general");
    state.dashboard=await api("/api/me/dashboard");
    if(!localStorage.getItem("calden_view_days")&&[1,7,14,30].includes(Number(state.settings.default_view)))state.viewDays=Number(state.settings.default_view);
    await reloadSharedData();
    renderApp();
    await applyCalDenRoute(false);
    if(state.me?.is_child && !(state.me.child_modules||[]).includes("calendar") && location.pathname==="/"){
      navigate("dashboard",true,true);
    }
    caldenBooting=false;
    if(location.pathname==="/")history.replaceState({calden:true},"",caldenPageURL("calendar"));
    startCalDenVersionWatch();
  }catch(err){
    console.error("Calden startup failed",err);reportCalDenClientError(err);
    if(err.status===401||err.status===403){
      setToken("");showAuth("login");
    }else{
      $("#app").classList.add("hidden");
      $("#auth").classList.remove("hidden");
      showBootFailure(err);
    }
    caldenBooting=false;
  }
}

async function reloadSharedData(){
  const [users,calendars,categories,notificationData]=await Promise.all([
    api("/api/users"),
    (!state.me?.is_child||(state.me.child_modules||[]).includes("calendar"))?api("/api/calendars"):Promise.resolve([]),
    (!state.me?.is_child||(state.me.child_modules||[]).includes("calendar"))?api("/api/categories"):Promise.resolve([]),
    (!state.me?.is_child||(state.me.child_modules||[]).includes("notifications"))?api("/api/notifications"):Promise.resolve({items:[],unread:0})
  ]);
  state.users=users;state.calendars=calendars;state.categories=categories;
  state.notifications=notificationData?.items||[];
  state.unreadNotifications=Number(notificationData?.unread)||0;
  state.notificationKnown=new Set(state.notifications.map(item=>item.id));
  if(!state.me?.is_child||(state.me.child_modules||[]).includes("calendar"))await loadEvents();
  else state.events=[];
}
async function loadEvents(){
  const from=startOfDay(addDays(state.anchorDate,-30)),to=addDays(from,365);
  state.events=await api(`/api/events?from=${encodeURIComponent(from.toISOString())}&to=${encodeURIComponent(to.toISOString())}`);
}

function renderApp(){
  hideBootStatus();$("#auth").classList.add("hidden");$("#app").classList.remove("hidden");
  $("#household-label").textContent=state.settings?.household_name||"";
  $("#sidebar-name").textContent=state.me.display_name;
  $("#sidebar-role").textContent=state.me.is_child?"Child account":roleLabel(state.me.role);
  $("#sidebar-avatar").innerHTML=state.me.avatar_url?`<img src="${escapeAttr(state.me.avatar_url)}" alt="">`:escapeHTML(state.me.initials||"?");
  $$(".admin-only").forEach(el=>el.classList.toggle("hidden",state.me.role!=="admin"));
  $$(".calendar-manager").forEach(el=>el.classList.toggle("hidden",!["admin","planner"].includes(state.me.role)));
  $$(".admin-settings").forEach(el=>el.classList.toggle("hidden",state.me.role!=="admin"));
  $("#settings-account-name").textContent=state.me.display_name;
  $("#settings-account-username").textContent=state.me.username;
  $("#settings-account-role").textContent=roleLabel(state.me.role);
  setAvatarPreview($("#settings-avatar-preview"),state.me);
  $("#remove-settings-avatar")?.classList.toggle("hidden",!state.me.avatar_url);
  renderEventControls();
  renderBillNavigation();
  renderCalendar();
  renderBills();
  renderAgenda();
  renderPeople();
  renderCalendars();
  renderCategories();
  renderNotifications();
  renderSettings();
  renderChores();
  window.caldenPackages?.renderNav();
  if(state.settings?.packages_enabled===true)window.caldenPackages?.load();
  $$(".shopping-nav").forEach(el=>el.classList.toggle("hidden",state.settings?.shopping_enabled===false));
  $$(".meals-nav").forEach(el=>el.classList.toggle("hidden",state.settings?.meals_enabled===false||state.me?.is_child||state.me?.role==="restricted"));
      $$(".routines-nav").forEach(el=>el.classList.toggle("hidden",state.settings?.routines_enabled!==true));
  window.caldenFamily?.renderDashboard();
  window.caldenFamily?.renderShopping();
  window.caldenRoutines?.render();
  window.caldenRewards?.render();
  window.caldenCare?.renderNav();
  $$(".rewards-nav").forEach(el=>el.classList.toggle("hidden",state.settings?.rewards_enabled!==true));
  window.caldenChildren?.applyChildView();
  decorateNavigation();
  window.caldenAppearance?.load();
  ensureNotificationPolling();
}



const caldenClientVersion=document.querySelector('meta[name="calden-version"]')?.content||"";
let caldenVersionWatch=null;
let caldenRefreshing=false;
async function checkCalDenClientVersion(){
  if(!state.me||!caldenClientVersion||caldenRefreshing)return;
  try{
    const response=await fetch("/api/health?fresh="+Date.now(),{cache:"no-store",headers:{"Cache-Control":"no-cache"}});
    if(!response.ok)return;
    const info=await response.json();
    if(!info.version||info.version===caldenClientVersion)return;
    const target=String(info.version);
    if(sessionStorage.getItem("calden_version_reload_attempt")===target)return;
    sessionStorage.setItem("calden_version_reload_attempt",target);
    caldenRefreshing=true;
    const next=new URL(location.href);
    next.searchParams.set("_calden_version",info.version);
    location.replace(next.pathname+next.search+next.hash);
  }catch(error){console.warn("Calden version check unavailable",error)}
}
function startCalDenVersionWatch(){
  if(caldenVersionWatch!==null)return;
  caldenVersionWatch=setInterval(checkCalDenClientVersion,30000);
  document.addEventListener("visibilitychange",()=>{if(!document.hidden)checkCalDenClientVersion()});
}

const caldenPages=new Set(["dashboard","shopping","meals","routines","rewards","school","maintenance","pets","calendar","bills","agenda","chores","people","calendars","categories","notifications","integrations","updates","backups","activity","settings","packages"]);
function caldenPageURL(page){
  if(page==="calendar")return "/calendar?date="+dateInputValue(state.anchorDate)+"&days="+state.viewDays;
  if(page==="bills")return "/bills?month="+dateInputValue(state.billMonth).slice(0,7);
  if(page==="settings")return "/settings/"+encodeURIComponent(state.settingsTab);
  return "/"+page;
}
function caldenEventURL(event){
  const prefix=isBillCalendar(event.calendar_id)?"/bills/":"/events/";
  return prefix+encodeURIComponent(event.id)+"?at="+encodeURIComponent(billOccurrenceStart(event));
}
function readCalDenRoute(){
  const parts=location.pathname.split("/").filter(Boolean);
  const params=new URLSearchParams(location.search);
  const detail=parts.length===2&&(parts[0]==="events"||parts[0]==="bills");
  const settingsAliases=new Set(["integrations","updates","backups"]);
  const page=detail?(parts[0]==="bills"?"bills":"calendar"):settingsAliases.has(parts[0])?"settings":caldenPages.has(parts[0])?parts[0]:"calendar";
  const at=params.get("at");
  const occurrence=at?new Date(at):null;
  const day=params.get("date");
  const date=day&&/^\d{4}-\d{2}-\d{2}$/.test(day)?calendarDate(day):occurrence&&!Number.isNaN(occurrence.getTime())?occurrence:null;
  const monthValue=params.get("month");
  const month=monthValue&&/^\d{4}-\d{2}$/.test(monthValue)?calendarDate(monthValue+"-01"):null;
  const tab=page==="settings"&&["general","calendar","optional-features","account","integrations","updates","backups","devices"].includes(parts[1])?parts[1]:settingsAliases.has(parts[0])?parts[0]:null;
  const days=Number(params.get("days"));
  return {page,detail,id:detail?parts[1]:null,kind:parts[0],at,date,month,tab,days};
}
function sameCaldenOccurrence(event,at){
  if(!at)return true;
  const found=new Date(billOccurrenceStart(event)).getTime(),target=new Date(at).getTime();
  return Number.isFinite(found)&&Number.isFinite(target)&&found===target;
}
async function applyCalDenRoute(load=true){
  const route=readCalDenRoute();
  if([1,7,14,30].includes(route.days))state.viewDays=route.days;
  if(route.date){state.anchorDate=startOfDay(route.date);if(load)await loadEvents()}
  if(route.month)state.billMonth=route.month;
  if(route.tab)activateSettingsTab(route.tab);
  navigate(route.page,load,false);
  if(route.page==="bills"&&!route.detail&&!load)await loadBillMonth();
  if($("#event-details-dialog").open){state.suppressDetailCloseRoute=true;$("#event-details-dialog").close()}
  state.detailEvent=null;
  document.querySelector(".calden-route-not-found")?.remove();
  if(!route.detail)return;
  let match=state.events.find(event=>event.id===route.id&&sameCaldenOccurrence(event,route.at));
  if(!match&&route.kind==="bills"){
    await loadBillMonth();
    match=state.billEvents.find(event=>event.id===route.id&&sameCaldenOccurrence(event,route.at));
  }
  if(!match&&route.date){
    const from=addDays(startOfDay(route.date),-2),to=addDays(startOfDay(route.date),3);
    const events=await api("/api/events?from="+encodeURIComponent(from.toISOString())+"&to="+encodeURIComponent(to.toISOString()));
    match=events.find(event=>event.id===route.id&&sameCaldenOccurrence(event,route.at));
  }
  if(match&&(route.kind!=="bills"||isBillCalendar(match.calendar_id))){
    openEventDetails(match,false);
  }else{
    const region=$("#page-"+route.page);
    const notice=document.createElement("p");
    notice.className="status-line calden-route-not-found";
    notice.textContent="This event or bill could not be found, or you do not have permission to view it.";
    region?.prepend(notice);
  }
}
window.addEventListener("popstate",()=>{if(state.me)applyCalDenRoute(true).catch(console.error)});

function navigate(page,load=true,writeURL=true){
  if(!caldenPages.has(page))page="calendar";
  const adminPages=new Set(["people","integrations","updates","backups","activity"]);
  if(adminPages.has(page)&&state.me?.role!=="admin")page="calendar";
  if(["calendars","categories"].includes(page)&&!["admin","planner"].includes(state.me?.role))page="calendar";
  if(page==="chores"&&!state.settings?.chores_enabled&&!state.settings?.tasks_enabled)page="calendar";
  if(page==="packages"&&(state.settings?.packages_enabled!==true||state.me?.is_child))page="calendar";
  if(page==="shopping"&&state.settings?.shopping_enabled===false)page="calendar";
  if(page==="meals"&&(state.settings?.meals_enabled===false||state.me?.is_child||state.me?.role==="restricted"))page="dashboard";
  if(page==="routines"&&state.settings?.routines_enabled!==true)page="calendar";
  if(page==="rewards"&&state.settings?.rewards_enabled!==true)page="calendar";
  if(["school","maintenance","pets"].includes(page)&&state.settings?.[page+"_enabled"]!==true)page="calendar";
  if(page==="agenda"&&state.settings?.agenda_enabled===false)page="calendar";
  if(page==="bills"&&state.settings?.bills_enabled===false)page="calendar";
  if(state.me?.is_child){
    const allowed=new Set(state.me.child_modules||[]);
    const permitted=page==="dashboard"||(page==="notifications"&&allowed.has("notifications"))||
      (page==="calendar"&&allowed.has("calendar"))||
      (page==="agenda"&&allowed.has("calendar"))||
      (page==="chores"&&(allowed.has("chores")||allowed.has("tasks")))||
      (page==="routines"&&allowed.has("routines"))||
      (page==="rewards"&&allowed.has("rewards"))||
      (page==="shopping"&&allowed.has("shopping"));
    if(!permitted)page="dashboard";
  }
  state.currentPage=page;
  if(writeURL)history.pushState({calden:true},"",caldenPageURL(page));
  $$(".app-page").forEach(el=>el.classList.toggle("hidden",el.id!==`page-${page}`));
  $$("[data-page]").forEach(el=>el.classList.toggle("active",el.dataset.page===page));
  const titles={dashboard:"My home",shopping:"Shopping",packages:"Packages",routines:"Routines",rewards:"Rewards & Allowances",school:"School",maintenance:"Home upkeep",pets:"Pets",calendar:"Calendar",bills:"Bill Pay",agenda:"Agenda",chores:"Tasks & Chores",people:"People",calendars:"Calendars",categories:"Categories",notifications:"Notifications",integrations:"Integrations",updates:"Updates",backups:"Backups & Restore",activity:"Activity",settings:"Settings"};
  $("#page-title").textContent=window.caldenAppearance?.label(page,titles[page]||"Calden")||titles[page]||"Calden";
  $("#new-event").classList.toggle("hidden",state.me?.is_child||!["calendar","agenda"].includes(page));
  $("#sidebar").classList.remove("open");
  window.caldenChildren?.applyChildView();
  window.caldenFamily?.enter(page);
  window.caldenRoutines?.enter(page);
  window.caldenRewards?.enter(page);
  window.caldenChildren?.enter(page);
  window.caldenCare?.enter(page);
  window.caldenPackages?.enter(page);
  window.caldenMeals?.enter(page);
  if(!load)return;
  if(page==="notifications")loadNotifications(false).catch(()=>renderNotifications());
  if(page==="bills")loadBillMonth();
  if(page==="chores")loadChores();
  if(page==="people")loadHouseholdDevices();
  if(page==="settings"){activateSettingsTab(state.settingsTab);window.caldenPersonalNotifications?.load()}
  if(page==="activity")loadActivity();
}

function renderEventControls(){
  const editable=state.calendars.filter(c=>c.can_edit);
  $("#calendar-select").innerHTML='<option value="">Choose a calendar</option>'+editable.map(c=>`<option value="${c.id}">${escapeHTML(c.name)}</option>`).join("");
  $("#category-select").innerHTML='<option value="">No category</option>'+state.categories.map(cat=>`<option value="${cat.id}">${escapeHTML(cat.name)}</option>`).join("");
  const activeUsers=state.users.filter(u=>u.active!==false);
  $("#people-picker").innerHTML=activeUsers.map(personChoice).join("");
  const billPayer=$("#bill-payer-select");
  if(billPayer){
    const selected=billPayer.value;
    billPayer.innerHTML='<option value="">Not assigned</option>'+activeUsers.map(u=>`<option value="${u.id}">${escapeHTML(u.display_name)}</option>`).join("");
    if(activeUsers.some(u=>u.id===selected))billPayer.value=selected;
  }

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
  const firstDay=(state.me?.week_start||state.settings?.week_start)==="monday"?1:0;
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
  // Calendar ranges are half-open: [start, end). iCalendar DTEND is exclusive,
  // so an all-day event ending at midnight must not render again on that end date.
  return visibleEvents().filter(e=>eventStartDate(e)<end&&eventEndDate(e)>start);
}
function renderCalendarFilters(){
  const category=$("#calendar-category-filter"),person=$("#calendar-person-filter"),search=$("#calendar-search");
  if(category)category.value=state.filters.category;
  if(person)person.value=state.filters.person;
  if(search&&search.value!==state.filters.query)search.value=state.filters.query;
  $("#clear-calendar-filters")?.classList.toggle("hidden",!state.filters.category&&!state.filters.person&&!state.filters.query);
}

function renderBillNavigation(){
  const visible=state.settings?.bills_enabled!==false && state.calendars.some(cal=>cal.calendar_type==="bill_pay");
  document.querySelectorAll(".bill-nav").forEach(el=>el.classList.toggle("hidden",!visible));
  document.querySelectorAll(".agenda-nav").forEach(el=>el.classList.toggle("hidden",state.settings?.agenda_enabled===false));
  if(!visible&&state.currentPage==="bills")navigate("calendar",false);
  if(state.settings?.agenda_enabled===false&&state.currentPage==="agenda")navigate("calendar",false);
}

async function loadBillMonth(){
  const start=new Date(state.billMonth.getFullYear(),state.billMonth.getMonth(),1);
  const end=new Date(state.billMonth.getFullYear(),state.billMonth.getMonth()+1,1);
  try{
    const events=await api(`/api/events?from=${encodeURIComponent(start.toISOString())}&to=${encodeURIComponent(end.toISOString())}`);
    state.billEvents=events.filter(event=>isBillCalendar(event.calendar_id));
    renderBills();
  }catch(err){
    const host=$("#bill-list");
    if(host)host.innerHTML=`<div class="empty-state"><strong>Could not load bills</strong><span>${escapeHTML(err.message)}</span></div>`;
  }
}

function billGroupRows(events,keyFn,labelFn){
  const groups=new Map();
  events.forEach(event=>{
    const key=keyFn(event),label=labelFn(event);
    if(!groups.has(key))groups.set(key,{label,total:0,known:0,estimated:0,count:0,unpriced:0});
    const group=groups.get(key);group.count++;
    if(billPaymentStatus(event)==="no_balance"){group.noBalance=(group.noBalance||0)+1;return}
    if(event.bill_amount===null||event.bill_amount===undefined){group.unpriced++;return}
    const amount=Number(event.bill_amount)||0;group.total+=amount;
    if(event.bill_amount_is_estimate)group.estimated+=amount;else group.known+=amount;
  });
  return [...groups.values()].sort((a,b)=>b.total-a.total||a.label.localeCompare(b.label));
}

function billBreakdownMarkup(rows){
  if(!rows.length)return '<div class="bill-empty-small">No bills in this month.</div>';
  return rows.map(row=>`<div class="bill-breakdown-row">
    <div><strong>${escapeHTML(row.label)}</strong><small>${row.count} bill${row.count===1?"":"s"}${row.noBalance?" · "+row.noBalance+" no balance":""}${row.unpriced?" · "+row.unpriced+" without amount":""}</small></div>
    <div><strong>${money(row.total)}</strong>${row.estimated?`<small>${money(row.estimated)} estimated</small>`:""}</div>
  </div>`).join("");
}

function billPaidRows(events){
  const groups=new Map();
  events.forEach(event=>{
    billPayments(event).forEach(payment=>{
      const key=payment.paid_by?.id||"__unknown__";
      const label=payment.paid_by?.display_name||"Unknown payer";
      if(!groups.has(key))groups.set(key,{label,total:0,count:0,unpriced:0});
      const group=groups.get(key);group.count++;group.total+=Number(payment.amount_paid)||0;
    });
  });
  return [...groups.values()].sort((a,b)=>b.total-a.total||a.label.localeCompare(b.label));
}

function billPaidBreakdownMarkup(rows){
  if(!rows.length)return '<div class="bill-empty-small">No payments recorded this month.</div>';
  return rows.map(row=>`<div class="bill-breakdown-row paid-breakdown-row">
    <div><strong>${escapeHTML(row.label)}</strong><small>${row.count} payment${row.count===1?"":"s"}</small></div>
    <div><strong>${money(row.total)}</strong><small>actually paid</small></div>
  </div>`).join("");
}

function billRemaining(event){
  const status=billPaymentStatus(event);
  if(status==="paid"||status==="cleared"||status==="no_balance")return 0;
  if(event.bill_amount===null||event.bill_amount===undefined)return null;
  return Math.max(0,(Number(event.bill_amount)||0)-billPaidAmount(event));
}

function renderBills(){
  const host=$("#bill-list"),summary=$("#bill-summary");
  if(!host||!summary)return;
  const billCalendars=state.calendars.filter(cal=>cal.calendar_type==="bill_pay");
  renderBillNavigation();
  if(!billCalendars.length){
    $("#bill-month-label").textContent="";
    summary.innerHTML="";
    $("#bill-by-person").innerHTML='<div class="bill-empty-small">No bill pay calendars are visible to you.</div>';
    $("#bill-by-calendar").innerHTML='<div class="bill-empty-small">No bill pay calendars are visible to you.</div>';
    $("#bill-paid-by-person").innerHTML='<div class="bill-empty-small">No bill pay calendars are visible to you.</div>';
    host.innerHTML='<div class="empty-state"><strong>No Bill Pay calendar</strong><span>An administrator can create a Bill Pay calendar and give you access.</span></div>';
    $("#bill-count").textContent="";
    return;
  }

  const start=new Date(state.billMonth.getFullYear(),state.billMonth.getMonth(),1);
  $("#bill-month-label").textContent=formatDate(start,{month:"long",year:"numeric"});
  const fallback=state.events.filter(event=>{
    const d=eventStartDate(event);
    return isBillCalendar(event.calendar_id)&&d.getFullYear()===start.getFullYear()&&d.getMonth()===start.getMonth();
  });
  const events=(state.currentPage==="bills"?state.billEvents:fallback).slice().sort((a,b)=>eventStartDate(a)-eventStartDate(b));

  let known=0,estimated=0,unpriced=0,paidTotal=0,outstanding=0;
  let allocatedCount=0,paidCount=0,clearedCount=0,partialCount=0,noBalanceCount=0,dueCount=0;
  events.forEach(event=>{
    const status=billPaymentStatus(event);
    const dueAmount=event.bill_amount===null||event.bill_amount===undefined?null:Number(event.bill_amount)||0;
    if(status!=="no_balance"){
      if(dueAmount===null)unpriced++;
      else if(event.bill_amount_is_estimate)estimated+=dueAmount;
      else known+=dueAmount;
    }
    paidTotal+=billPaidAmount(event);

    if(status==="allocated")allocatedCount++;
    else if(status==="paid")paidCount++;
    else if(status==="cleared")clearedCount++;
    else if(status==="partial")partialCount++;
    else if(status==="no_balance")noBalanceCount++;
    else dueCount++;
    const remaining=billRemaining(event);
    if(remaining!==null)outstanding+=remaining;
  });
  const total=known+estimated;
  summary.innerHTML=`
    <article class="bill-stat"><span>Expected this month</span><strong>${money(total)}</strong><small>known + estimated bills</small></article>
    <article class="bill-stat"><span>Known amounts</span><strong>${money(known)}</strong><small>fixed or confirmed amounts</small></article>
    <article class="bill-stat"><span>Estimated</span><strong>${money(estimated)}</strong><small>variable bills marked as estimates</small></article>
    <article class="bill-stat paid-stat"><span>Paid so far</span><strong>${money(paidTotal)}</strong><small>${clearedCount} cleared · ${paidCount} paid · ${partialCount} partial</small></article>
    <article class="bill-stat outstanding-stat"><span>Outstanding</span><strong>${money(outstanding)}</strong><small>${dueCount+allocatedCount+partialCount} bill${dueCount+allocatedCount+partialCount===1?"":"s"} still open${unpriced?" · "+unpriced+" without amount":""}</small></article>`;

  const byPerson=billGroupRows(events,event=>event.bill_payer?.id||"__unassigned__",event=>event.bill_payer?.display_name||"Unassigned");
  const byCalendar=billGroupRows(events,event=>event.calendar_id,event=>event.calendar_name||"Bill calendar");
  const paidByPerson=billPaidRows(events);
  $("#bill-by-person").innerHTML=billBreakdownMarkup(byPerson);
  $("#bill-by-calendar").innerHTML=billBreakdownMarkup(byCalendar);
  $("#bill-paid-by-person").innerHTML=billPaidBreakdownMarkup(paidByPerson);
  $("#bill-count").textContent=`${clearedCount} cleared · ${paidCount} paid · ${allocatedCount} allocated · ${partialCount} partial · ${noBalanceCount} no balance · ${dueCount} due`;

  host.innerHTML=events.length?events.map(event=>{
    const due=eventStartDate(event),amount=billAmountLabel(event);
    const payer=event.bill_payer?.display_name||"Not assigned";
    const status=billPaymentStatus(event);
    const paidAmount=billPaidAmount(event),remaining=billRemaining(event);
    const payments=billPayments(event);
    const last=payments[payments.length-1];
    const allocatedAmount=billAllocatedAmount(event);
    const allocatedOn=event.bill_allocated_on?formatDate(new Date(event.bill_allocated_on+"T12:00:00"),{month:"short",day:"numeric"}):"";
    const paymentState=status==="cleared"
      ?`<span class="bill-paid-status"><span class="bill-status-pill cleared">Cleared</span><strong>${money(paidAmount)}</strong><small>${last?.cleared_on?"Cleared "+escapeHTML(formatDate(new Date(last.cleared_on+"T12:00:00"),{month:"short",day:"numeric"})):"Payment cleared"}</small></span>`
      :status==="paid"
        ?`<span class="bill-paid-status"><span class="bill-status-pill paid">Paid</span><strong>${money(paidAmount)}</strong><small>${last?.paid_on?"Paid "+escapeHTML(formatDate(new Date(last.paid_on+"T12:00:00"),{month:"short",day:"numeric"})):"Payment recorded"} · waiting to clear</small></span>`
        :status==="partial"
          ?`<span class="bill-paid-status"><span class="bill-status-pill partial">Partial</span><strong>${money(paidAmount)} paid</strong><small>${payments.length} payment${payments.length===1?"":"s"}${remaining===null?"":" · "+money(remaining)+" remaining"}</small></span>`
          :status==="allocated"
            ?`<span class="bill-paid-status"><span class="bill-status-pill allocated">Allocated</span><strong>${money(allocatedAmount)}</strong><small>${allocatedOn?"Moved "+escapeHTML(allocatedOn):"Funds moved to bill-pay"}</small></span>`
            :status==="no_balance"
              ?`<span class="bill-paid-status"><span class="bill-status-pill no-balance">No balance</span><strong>${money(0)}</strong><small>Nothing due this month</small></span>`
              :`<span class="bill-paid-status"><span class="bill-status-pill due">Due</span><strong>Not paid yet</strong><small>Assigned to ${escapeHTML(payer)}</small></span>`;
    return `<article class="bill-row is-${status}">
      <button type="button" class="bill-row-edit" data-event-key="${escapeAttr(eventKey(event))}" aria-label="View details for ${escapeAttr(event.title)}">
        <span class="bill-due"><strong>${formatDate(due,{month:"short",day:"numeric"})}</strong><small>${event.all_day?"Due date":formatTime(due)}</small></span>
        <span class="bill-row-main"><strong>${escapeHTML(event.title)}</strong><small>${escapeHTML(event.calendar_name||"Bills")} · assigned ${escapeHTML(payer)}</small></span>
        <span class="bill-row-amount ${event.bill_amount_is_estimate?"estimated":""}"><strong>${amount?escapeHTML(amount):"Amount not set"}</strong><small>${event.bill_amount_is_estimate?"Estimated":"Amount due"}</small></span>
      </button>
      <span class="bill-row-payment">${paymentState}${canUpdateBill(event)?`<button type="button" class="button ${status==="due"?"":"secondary"} compact bill-payment-action" data-event-key="${escapeAttr(eventKey(event))}">${status==="due"?"Manage":"Activity"}</button>`:""}</span>
    </article>`;
  }).join(""):'<div class="empty-state"><strong>Nothing due this month</strong><span>Add a bill or move to another month.</span></div>';

  host.querySelectorAll(".bill-row-edit[data-event-key]").forEach(button=>button.addEventListener("click",()=>{
    const event=events.find(item=>eventKey(item)===button.dataset.eventKey);
    if(event)openEventDetails(event);
  }));
  host.querySelectorAll(".bill-payment-action[data-event-key]").forEach(button=>button.addEventListener("click",e=>{
    e.stopPropagation();
    const item=events.find(entry=>eventKey(entry)===button.dataset.eventKey);
    if(item)openBillPayment(item);
  }));
}

function resetBillPaymentEntry(event=state.billPaymentEvent){
  state.billPaymentEditingId=null;
  $("#bill-payment-entry-title").textContent="Add a payment";
  $("#bill-payment-save").textContent="Add payment";
  $("#bill-payment-reset").classList.add("hidden");
  const remaining=event?billRemaining(event):null;
  $("#bill-payment-amount").value=remaining!==null&&remaining>0?String(Math.round(remaining*100)/100):"";
  $("#bill-payment-date").value=dateInputValue();
  $("#bill-cleared-date").value="";
  $("#bill-payment-settles").checked=remaining!==null&&remaining>0;
  const preferred=event?.bill_payer?.id||state.me?.id||state.users.find(user=>user.active!==false)?.id||"";
  if(preferred)$("#bill-payment-person").value=preferred;
  $("#bill-payment-error").textContent="";
}

function renderBillPaymentModal(){
  const event=state.billPaymentEvent;if(!event)return;
  const due=eventStartDate(event),assigned=event.bill_payer?.display_name||"Not assigned";
  const dueAmount=event.bill_amount===null||event.bill_amount===undefined?null:Number(event.bill_amount)||0;
  const paidAmount=billPaidAmount(event),allocatedAmount=billAllocatedAmount(event),remaining=billRemaining(event),status=billPaymentStatus(event);
  $("#bill-payment-title").textContent="Bill activity";
  $("#bill-payment-copy").textContent=`${event.title} · due ${formatDate(due,{month:"long",day:"numeric",year:"numeric"})}`;
  $("#bill-payment-assignment").innerHTML=`<span>Originally assigned</span><strong>${escapeHTML(assigned)}</strong><span>Due date</span><strong>${escapeHTML(formatDate(due,{month:"short",day:"numeric",year:"numeric"}))}</strong>${event.bill_amount_is_estimate?'<small>Expected amount is currently an estimate.</small>':""}`;
  $("#bill-payment-summary").innerHTML=`
    <div><span>Due</span><strong>${dueAmount===null?"Not set":money(dueAmount)}</strong></div>
    <div><span>Allocated</span><strong>${allocatedAmount?money(allocatedAmount):money(0)}</strong></div>
    <div><span>Paid</span><strong>${money(paidAmount)}</strong></div>
    <div><span>Remaining</span><strong>${remaining===null?"Unknown":money(remaining)}</strong></div>
    <div><span>Status</span><strong>${billStatusLabel(status)}</strong></div>`;

  const allocationCurrent=$("#bill-allocation-current");
  if(allocatedAmount>0){
    const allocatedDate=event.bill_allocated_on?formatDate(new Date(event.bill_allocated_on+"T12:00:00"),{month:"short",day:"numeric",year:"numeric"}):"date not recorded";
    const allocatedBy=event.bill_allocated_by?.display_name||"Household";
    allocationCurrent.classList.remove("hidden");
    allocationCurrent.innerHTML=`<strong>${money(allocatedAmount)} allocated</strong><span>${escapeHTML(allocatedBy)} · ${escapeHTML(allocatedDate)}</span>`;
    $("#bill-allocation-amount").value=String(Math.round(allocatedAmount*100)/100);
    $("#bill-allocation-date").value=event.bill_allocated_on||dateInputValue();
    $("#bill-allocation-save").textContent="Update allocation";
    $("#bill-allocation-clear").classList.remove("hidden");
  }else{
    allocationCurrent.classList.add("hidden");
    allocationCurrent.innerHTML="";
    $("#bill-allocation-amount").value=dueAmount!==null&&dueAmount>0?String(Math.round(dueAmount*100)/100):"";
    $("#bill-allocation-date").value=dateInputValue();
    $("#bill-allocation-save").textContent="Mark allocated";
    $("#bill-allocation-clear").classList.add("hidden");
  }
  $("#bill-allocation-error").textContent="";

  const history=$("#bill-payment-history"),payments=billPayments(event);
  history.innerHTML=payments.length?payments.map(payment=>{
    const person=payment.paid_by?.display_name||"Unknown payer";
    const paidOn=payment.paid_on?formatDate(new Date(payment.paid_on+"T12:00:00"),{month:"short",day:"numeric",year:"numeric"}):"Date unknown";
    const cleared=payment.cleared_on?formatDate(new Date(payment.cleared_on+"T12:00:00"),{month:"short",day:"numeric",year:"numeric"}):"Not marked cleared";
    return `<article class="bill-payment-history-row">
      <div><strong>${money(payment.amount_paid)}</strong><span>${escapeHTML(person)}</span></div>
      <div><span>Paid ${escapeHTML(paidOn)}</span><small>${escapeHTML(cleared)}</small></div>
      <span class="bill-status-pill ${payment.cleared_on?"cleared":payment.settles_bill?"paid":"partial"}">${payment.cleared_on?"Cleared":payment.settles_bill?"Final":"Partial"}</span>
      <div class="bill-payment-history-actions" role="group" aria-label="Actions for payment of ${escapeAttr(money(payment.amount_paid))}">
        ${!payment.cleared_on?`<button type="button" class="button secondary compact clear-bill-payment" data-payment-id="${escapeAttr(payment.id)}" aria-label="Mark payment of ${escapeAttr(money(payment.amount_paid))} cleared">Mark cleared</button>`:""}
        <button type="button" class="button secondary compact edit-bill-payment" data-payment-id="${escapeAttr(payment.id)}" aria-label="Edit payment of ${escapeAttr(money(payment.amount_paid))}">Edit</button>
        <button type="button" class="button danger compact delete-bill-payment" data-payment-id="${escapeAttr(payment.id)}" aria-label="Delete payment of ${escapeAttr(money(payment.amount_paid))}">Delete</button>
      </div>
    </article>`;
  }).join(""):status==="no_balance"
    ?'<div class="bill-no-balance-state"><strong>No balance this month</strong><span>No payment was required for this occurrence.</span></div>'
    :'<div class="bill-empty-small">No payments recorded for this month.</div>';

  $("#bill-payment-no-balance").textContent=status==="no_balance"?"Reopen bill":"No balance this month";
  history.querySelectorAll(".clear-bill-payment").forEach(button=>button.addEventListener("click",()=>markBillPaymentCleared(button.dataset.paymentId)));
  history.querySelectorAll(".edit-bill-payment").forEach(button=>button.addEventListener("click",()=>editBillPayment(button.dataset.paymentId)));
  history.querySelectorAll(".delete-bill-payment").forEach(button=>button.addEventListener("click",()=>deleteBillPaymentEntry(button.dataset.paymentId)));
}

function openBillPayment(event){
  if(!event||!canUpdateBill(event))return;
  state.billPaymentEvent=event;
  state.billPaymentEditingId=null;
  const users=state.users.filter(user=>user.active!==false);
  $("#bill-payment-person").innerHTML=users.map(user=>`<option value="${user.id}">${escapeHTML(user.display_name)}</option>`).join("");
  renderBillPaymentModal();
  resetBillPaymentEntry(event);
  $("#bill-payment-dialog").showModal();
}

function editBillPayment(paymentID){
  const event=state.billPaymentEvent;
  if(!event)return;
  const payment=billPayments(event).find(item=>item.id===paymentID);
  if(!payment)return;
  // A former payer might be inactive now; retain their ID while editing
  // the historical entry rather than silently attributing it to someone else.
  if(payment.paid_by?.id&&!Array.from($("#bill-payment-person").options).some(option=>option.value===payment.paid_by.id)){
    $("#bill-payment-person").add(new Option(payment.paid_by.display_name||"Former household member",payment.paid_by.id));
  }
  state.billPaymentEditingId=paymentID;
  $("#bill-payment-entry-title").textContent="Edit payment";
  $("#bill-payment-save").textContent="Update payment";
  $("#bill-payment-reset").classList.remove("hidden");
  if(payment.paid_by?.id)$("#bill-payment-person").value=payment.paid_by.id;
  $("#bill-payment-amount").value=String(Number(payment.amount_paid)||0);
  $("#bill-payment-date").value=payment.paid_on||dateInputValue();
  $("#bill-cleared-date").value=payment.cleared_on||"";
  $("#bill-payment-settles").checked=!!payment.settles_bill;
  $("#bill-payment-error").textContent="";
}

async function refreshBillPaymentViews(keepOpen=false){
  await loadEvents();
  if(state.currentPage==="bills")await loadBillMonth();
  else renderBills();
  await loadNotifications(false);
  renderCalendar();renderAgenda();renderNotifications();
  if(keepOpen&&state.billPaymentEvent){
    const key=eventKey(state.billPaymentEvent);
    const updated=(state.currentPage==="bills"?state.billEvents:state.events).find(item=>eventKey(item)===key);
    if(updated){state.billPaymentEvent=updated;renderBillPaymentModal()}
  }
}

async function saveBillAllocation(){
  const event=state.billPaymentEvent;if(!event)return;
  const amount=Number($("#bill-allocation-amount").value);
  const allocatedOn=$("#bill-allocation-date").value;
  if(!Number.isFinite(amount)||amount<=0){$("#bill-allocation-error").textContent="Enter the amount allocated.";return}
  if(!allocatedOn){$("#bill-allocation-error").textContent="Choose the allocation date.";return}
  const button=$("#bill-allocation-save");button.disabled=true;$("#bill-allocation-error").textContent="";
  try{
    await api(`/api/bills/${event.id}/allocation`,{method:"PUT",body:JSON.stringify({
      occurrence_start:billOccurrenceStart(event),allocated:true,amount_allocated:amount,allocated_on:allocatedOn
    })});
    await refreshBillPaymentViews(true);
  }catch(err){$("#bill-allocation-error").textContent=err.message}
  finally{button.disabled=false}
}

async function clearBillAllocation(){
  const event=state.billPaymentEvent;if(!event)return;
  const button=$("#bill-allocation-clear");button.disabled=true;$("#bill-allocation-error").textContent="";
  try{
    await api(`/api/bills/${event.id}/allocation`,{method:"PUT",body:JSON.stringify({
      occurrence_start:billOccurrenceStart(event),allocated:false,amount_allocated:0,allocated_on:""
    })});
    await refreshBillPaymentViews(true);
  }catch(err){$("#bill-allocation-error").textContent=err.message}
  finally{button.disabled=false}
}

async function saveBillPaymentEntry(){
  const event=state.billPaymentEvent;if(!event)return;
  const amount=Number($("#bill-payment-amount").value);
  if(!Number.isFinite(amount)||amount<=0||amount>9999999999.99||Math.round(amount*100)/100!==amount){
    $("#bill-payment-error").textContent="Enter a valid payment amount with no more than two decimal places.";return;
  }
  const payload={
    occurrence_start:billOccurrenceStart(event),
    paid_by_user_id:$("#bill-payment-person").value,
    amount_paid:amount,
    paid_on:$("#bill-payment-date").value,
    cleared_on:$("#bill-cleared-date").value||"",
    settles_bill:$("#bill-payment-settles").checked
  };
  if(!payload.paid_on){$("#bill-payment-error").textContent="Choose the payment date.";return}
  if(payload.cleared_on&&payload.cleared_on<payload.paid_on){$("#bill-payment-error").textContent="Cleared date cannot be before the payment date.";return}
  const button=$("#bill-payment-save");button.disabled=true;$("#bill-payment-error").textContent="";
  try{
    const editing=state.billPaymentEditingId;
    const path=editing?`/api/bills/${event.id}/payments/${editing}`:`/api/bills/${event.id}/payments`;
    await api(path,{method:editing?"PUT":"POST",body:JSON.stringify(payload)});
    state.billPaymentEditingId=null;
    await refreshBillPaymentViews(true);
    resetBillPaymentEntry(state.billPaymentEvent);
  }catch(err){$("#bill-payment-error").textContent=err.message}
  finally{button.disabled=false}
}

async function markBillPaymentCleared(paymentID){
  const event=state.billPaymentEvent,payment=billPayments(event).find(item=>item.id===paymentID);
  if(!event||!payment)return;
  try{
    await api(`/api/bills/${event.id}/payments/${paymentID}`,{method:"PUT",body:JSON.stringify({
      occurrence_start:billOccurrenceStart(event),
      paid_by_user_id:payment.paid_by?.id||state.me?.id||"",
      amount_paid:Number(payment.amount_paid)||0,
      paid_on:payment.paid_on,
      cleared_on:dateInputValue(),
      settles_bill:!!payment.settles_bill
    })});
    await refreshBillPaymentViews(true);
    resetBillPaymentEntry(state.billPaymentEvent);
  }catch(err){$("#bill-payment-error").textContent=err.message}
}

async function deleteBillPaymentEntry(paymentID){
  const event=state.billPaymentEvent;if(!event)return;
  if(!confirm("Delete this payment entry? The bill itself will stay on the calendar."))return;
  try{
    await api(`/api/bills/${event.id}/payments/${paymentID}`,{method:"DELETE",body:JSON.stringify({occurrence_start:billOccurrenceStart(event)})});
    await refreshBillPaymentViews(true);
    resetBillPaymentEntry(state.billPaymentEvent);
  }catch(err){$("#bill-payment-error").textContent=err.message}
}

async function toggleBillNoBalance(){
  const event=state.billPaymentEvent;if(!event)return;
  const enable=billPaymentStatus(event)!=="no_balance";
  if(enable&&(billPayments(event).length||billAllocatedAmount(event)>0)&&!confirm("Mark no balance this month? Existing payment entries and allocated funds for this month will be removed."))return;
  try{
    await api(`/api/bills/${event.id}/no-balance`,{method:"PUT",body:JSON.stringify({occurrence_start:billOccurrenceStart(event),no_balance:enable})});
    await refreshBillPaymentViews(true);
    resetBillPaymentEntry(state.billPaymentEvent);
  }catch(err){$("#bill-payment-error").textContent=err.message}
}

function renderCalendar(){
  const label=$("#calendar-range-label");
  $$(".view-switcher button").forEach(b=>b.classList.toggle("active",Number(b.dataset.days)===state.viewDays));
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
    const allDayHTML=allDay.map(e=>calendarEventBlock(e,true)).join("")+(window.caldenPackages?.calendarForDay(day)||"");
    const timedHTML=layoutTimedEvents(timed,day).map(item=>{
      const top=clamp(item.startMin/totalMinutes*100,0,100);
      const height=clamp((item.endMin-item.startMin)/totalMinutes*100,1.4,100-top);
      const left=item.column/item.columns*100,width=100/item.columns;
      const e=item.event;
      return `<button class="timed-event" data-event-key="${escapeAttr(eventKey(e))}" style="--cal:${safeColor(e.calendar_color||e.color)};--top:${top}%;--height:${height}%;--left:${left}%;--width:${width}%">
        <strong>${billCalendarStatusMarker(e)}${escapeHTML(e.title)}</strong><span>${formatTime(new Date(e.starts_at))}${billAmountLabel(e)?" · "+escapeHTML(billAmountLabel(e)):""}${e.category_name?" · "+escapeHTML(e.category_name):""}</span>${isBillCalendar(e.calendar_id)?billPaidAvatar(e):avatarMini(e)}
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
    const day=addDays(start,i),events=eventsForDay(day).sort((a,b)=>eventStartDate(a)-eventStartDate(b));
    const visible=events.slice(0,state.viewDays===30?5:7);
    const more=events.length-visible.length;
    const monthMarker=i===0||day.getDate()===1?`<span class="month-marker">${formatDate(day,{month:"short"})}</span>`:"";
    return `<article class="day-cell ${sameDay(day,new Date())?"today":""} ${[0,6].includes(day.getDay())?"weekend":""}" data-date="${day.toISOString()}">
      <header><span>${monthMarker}</span><strong>${day.getDate()}</strong><button type="button" class="day-add" data-add-date="${escapeAttr(day.toISOString())}" aria-label="Add event on ${escapeAttr(formatDate(day,{month:"long",day:"numeric"}))}">+</button></header>
      <div class="day-events">${visible.map(e=>calendarEventBlock(e,false)).join("")}${window.caldenPackages?.calendarForDay(day)||""}${more>0?`<span class="more-events">+${more} more</span>`:""}</div>
    </article>`;
  }).join("");
  return `<div class="day-grid-shell"><div class="day-grid-weekdays">${weekdayHeader}</div><div class="day-grid" style="--grid-days:7">${cells}</div></div>`;
}

function billPaidAvatar(e){
  const who=e.bill_paid_by;
  if(!who||!["paid","cleared","partial"].includes(billPaymentStatus(e)))return "";
  return '<span class="bill-payment-avatar" title="Paid by '+escapeAttr(who.display_name||"Unknown")+'">'+
    (who.avatar_url?'<img src="'+escapeAttr(who.avatar_url)+'" alt="">':
      escapeHTML(who.initials||who.display_name?.slice(0,1)||"?"))+'</span>';
}
function calendarEventBlock(e,compact=false){
  const bill=isBillCalendar(e.calendar_id);
  const time=e.all_day?"All day":formatTime(new Date(e.starts_at));
  const meta=(bill?[billAmountLabel(e),e.category_name]:[time,e.category_name]).filter(Boolean).join(" · ");
  const paymentAvatar=bill?billPaidAvatar(e):"";
  return `<button class="calendar-event ${compact?"compact":""}" data-event-key="${escapeAttr(eventKey(e))}" style="--cal:${safeColor(e.calendar_color||e.color)}">
    <span class="event-color"></span><span class="calendar-event-copy"><strong>${e.is_recurring?'<span class="repeat-mark" title="Repeating event">↻</span> ':""}${billCalendarStatusMarker(e)}${escapeHTML(e.title)}</strong><small>${escapeHTML(meta)}</small></span>${bill?paymentAvatar:avatarMini(e)}
  </button>`;
}
function avatarMini(e){
  const people=e.assignees||[];
  if(!people.length)return "";
  return `<span class="mini-avatars">${people.slice(0,3).map(person=>`<i title="${escapeAttr(person.display_name)}">${person.avatar_url?`<img src="${escapeAttr(person.avatar_url)}" alt="">`:escapeHTML(person.initials)}</i>`).join("")}${people.length>3?`<i>+${people.length-3}</i>`:""}</span>`;
}
function bindCalendarEvents(){
  const host=$("#calendar-view");
  window.caldenPackages?.bindCalendar(host);
  host.querySelectorAll("[data-event-key]").forEach(el=>el.addEventListener("click",event=>{
    event.stopPropagation();
    const found=findEventByKey(el.dataset.eventKey);if(found)openEventDetails(found);
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
    if(host.scrollHeight>host.clientHeight)host.scrollTop=Math.max(0,minutes/(24*60)*1152-180);
  });
}

function renderAgenda(){
  const query=($("#agenda-search")?.value||"").trim().toLowerCase();
  const kind=$("#agenda-kind")?.value||"all";
  const calendarSelect=$("#agenda-calendar");
  const selectedCalendar=calendarSelect?.value||"";
  if(calendarSelect){
    calendarSelect.innerHTML='<option value="">All calendars</option>'+state.calendars.map(cal=>'<option value="'+escapeAttr(cal.id)+'">'+escapeHTML(cal.name)+'</option>').join("");
    calendarSelect.value=selectedCalendar;
  }
  const days=Number($("#agenda-range")?.value||30);
  const from=new Date(),until=addDays(startOfDay(from),days);
  const events=visibleEvents().filter(e=>{
    if(eventStartDate(e)>=until||eventEndDate(e)<from)return false;
    const bill=isBillCalendar(e.calendar_id);
    if(selectedCalendar&&e.calendar_id!==selectedCalendar)return false;
    if(kind==="bills"&&!bill||kind==="events"&&bill)return false;
    if(!query)return true;
    return [e.title,e.location,e.notes,e.calendar_name,...(e.assignees||[]).map(a=>a.display_name)].join(" ").toLowerCase().includes(query);
  }).sort((a,b)=>eventStartDate(a)-eventStartDate(b));
  $("#event-count").textContent=`${events.length} event${events.length===1?"":"s"}`;
  $("#events").innerHTML=events.length?events.map(eventCard).join(""):'<div class="empty-state"><strong>No matching events</strong><span>Add an event or change your filters.</span></div>';
  $("#events").querySelectorAll("[data-event-key]").forEach(el=>el.addEventListener("click",()=>{
    const ev=findEventByKey(el.dataset.eventKey);if(ev)openEventDetails(ev);
  }));
}
function eventCard(e){
  const start=eventStartDate(e),end=eventEndDate(e);
  return `<button class="agenda-event" data-event-key="${escapeAttr(eventKey(e))}" style="--cal:${safeColor(e.calendar_color||e.color)}">
    <span class="agenda-color"></span><span class="agenda-date"><strong>${formatDate(start,{month:"short",day:"numeric"})}</strong><small>${isBillCalendar(e.calendar_id)?"Due":e.all_day?"All day":formatTime(start)}</small></span>
    <span class="agenda-main"><strong>${isBillCalendar(e.calendar_id)?billCalendarStatusMarker(e):""}${e.is_recurring?'<span class="repeat-mark" title="Repeating event">↻</span> ':""}${escapeHTML(e.title)}${billAmountLabel(e)?` · ${escapeHTML(billAmountLabel(e))}`:""}</strong><small>${escapeHTML(e.calendar_name)}${e.location?" · "+escapeHTML(e.location):""}${e.all_day?"":` · ends ${escapeHTML(formatTime(end))}`}</small></span>
    ${isBillCalendar(e.calendar_id)?billPaidAvatar(e):avatarMini(e)}
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

function notificationBadge(){
  const badge=$("#notification-nav-badge");
  if(!badge)return;
  const count=Math.max(0,Number(state.unreadNotifications)||0);
  badge.textContent=count>99?"99+":String(count);
  badge.classList.toggle("hidden",count===0);
}

function browserNotificationButton(){
  const button=$("#enable-browser-notifications");
  if(!button)return;
  if(!("Notification" in window)){
    button.classList.add("hidden");
    return;
  }
  if(Notification.permission==="granted"){
    button.classList.add("hidden");
    return;
  }
  button.classList.remove("hidden");
  button.disabled=Notification.permission==="denied";
  button.textContent=Notification.permission==="denied"?"Browser alerts blocked":"Enable browser alerts";
}

function notificationEvent(item){
  if(!item?.event_id)return null;
  if(item.kind==="bill_review"&&item.occurrence_start){
    const target=new Date(item.occurrence_start).getTime();
    return state.events.find(event=>event.id===item.event_id&&new Date(billOccurrenceStart(event)).getTime()===target)
      ||state.events.find(event=>event.id===item.event_id)
      ||null;
  }
  return state.events.find(event=>event.id===item.event_id)||null;
}

function renderNotificationInbox(){
  const host=$("#notification-inbox-list");
  if(!host)return;
  notificationBadge();
  browserNotificationButton();
  const unread=Math.max(0,Number(state.unreadNotifications)||0);
  $("#notification-unread-count").textContent=unread?unread+" unread":"All caught up";
  $("#mark-notifications-read").disabled=unread===0;

  host.innerHTML=state.notifications.length?state.notifications.map(item=>{
    const billReview=item.kind==="bill_review";
    const careReminder=item.kind==="care_due"||item.kind==="care_advance";
    const startValue=(billReview||careReminder)&&item.occurrence_start?item.occurrence_start:item.starts_at;
    // Care occurrence timestamps identify household dates, not clock times.
    // Parsing as a local calendar date avoids displaying yesterday in US time zones.
    const start=startValue?(careReminder||item.all_day?calendarDate(startValue):new Date(startValue)):null;
    const when=start?(careReminder?"Scheduled "+formatDate(start,{weekday:"short",month:"short",day:"numeric",year:"numeric"}):
      (item.all_day?formatDate(start,{weekday:"short",month:"short",day:"numeric",year:"numeric"}):
        formatDate(start,{weekday:"short",month:"short",day:"numeric",year:"numeric"})+" · "+formatTime(start))):
      (careReminder?"Schedule date unavailable":"Event date unavailable");
    const family=item.kind==="event_family";
    const color=safeColor(item.calendar_color||"#64748b");
    const iconClass=billReview?"bill":family?"family":"assigned";
    const icon=billReview?"$":family?"F":careReminder?"✓":"Y";
    const careName={school:"School",maintenance:"Home upkeep",pets:"Pet care"}[item.care_kind]||"Family care";
    const kicker=careReminder?careName+(item.kind==="care_advance"?" · Coming up":" · Due today"):
      billReview?"Bill follow-up":family?"For everyone":"Assigned to you";
    return `<div class="notification-inbox-entry"><button type="button" class="notification-inbox-row ${item.read_at?"":"unread"}" data-notification-id="${escapeAttr(item.id)}">
      <span class="notification-inbox-icon ${iconClass}">${icon}</span>
      <span class="notification-inbox-main">
        <span class="notification-inbox-kicker">${escapeHTML(kicker)}${careReminder?"":" · "+escapeHTML(item.calendar_name||"Calendar")}</span>
        <strong>${escapeHTML(item.title)}</strong>
        <small>${escapeHTML(item.message)} ${escapeHTML(when)}</small>
      </span>
      <span class="notification-inbox-side"><i style="--cal:${color}"></i><time title="Notification received (not the event date)">Notified ${escapeHTML(formatDate(new Date(item.created_at),{month:"short",day:"numeric",year:"numeric"}))}</time></span>
    </button><button type="button" class="notification-dismiss" data-dismiss-notification="${escapeAttr(item.id)}" aria-label="Dismiss alert" title="Dismiss alert">×</button></div>`;
  }).join(""):'<div class="empty-state notification-empty"><strong>No alerts yet</strong><span>New assignments and whole-family events will appear here.</span></div>';

  host.querySelectorAll("[data-dismiss-notification]").forEach(button=>button.addEventListener("click",async()=>{
    button.disabled=true;
    try{
      await api("/api/notifications/"+encodeURIComponent(button.dataset.dismissNotification),{method:"DELETE"});
      state.notifications=state.notifications.filter(n=>n.id!==button.dataset.dismissNotification);
      state.unreadNotifications=state.notifications.filter(n=>!n.read_at).length;
      renderNotificationInbox();
    }catch(err){button.disabled=false;alert(err.message)}
  }));
  host.querySelectorAll("[data-notification-id]").forEach(button=>button.addEventListener("click",async()=>{
    const item=state.notifications.find(n=>n.id===button.dataset.notificationId);
    if(!item)return;
    if(!item.read_at){
      try{
        await api("/api/notifications/"+item.id+"/read",{method:"PUT"});
        item.read_at=new Date().toISOString();
        state.unreadNotifications=Math.max(0,state.unreadNotifications-1);
      }catch{}
      renderNotificationInbox();
    }
    if((item.kind==="care_due"||item.kind==="care_advance")&&
      item.care_kind&&item.care_subject_id){
      window.caldenCare?.open(item.care_kind,item.care_subject_id);
      return;
    }
    const event=notificationEvent(item);
    if(event)openEventDetails(event);
  }));
}

async function loadNotifications(announce=true){
  const previous=new Set(state.notificationKnown);
  const data=await api("/api/notifications");
  state.notifications=data?.items||[];
  state.unreadNotifications=Number(data?.unread)||0;

  if(announce&&"Notification" in window&&Notification.permission==="granted"){
    state.notifications.slice().reverse().forEach(item=>{
      if(item.read_at||previous.has(item.id))return;
      const family=item.kind==="event_family";
      const billReview=item.kind==="bill_review";
      const careReminder=item.kind==="care_due"||item.kind==="care_advance";
      const body=careReminder?item.title+" · "+item.message:
        billReview?item.message+" "+item.title:(family?"For everyone: ":"Assigned to you: ")+item.title;
      const title=careReminder?(item.kind==="care_advance"?"Upcoming family reminder":"Family care due today"):
        billReview?"Bill follow-up":family?"New family event":"New event assignment";
      try{new Notification("Calden · "+title,{body,tag:"calden-"+item.id})}catch{}
    });
  }
  state.notificationKnown=new Set(state.notifications.map(item=>item.id));
  renderNotifications();
}

let caldenNotificationSocket=null;
let caldenNotificationRetry=null;
function connectCaldenNotifications(){
  if(!state.token||caldenNotificationSocket||document.hidden)return;
  const token=state.token;
  const address=new URL("/api/notifications/stream",location.href);
  address.protocol=address.protocol==="https:"?"wss:":"ws:";
  try{
    const socket=new WebSocket(address.href,["calden","calden.auth."+token]);
    caldenNotificationSocket=socket;
    socket.onmessage=()=>{
      if(state.token===token)loadNotifications(true).catch(()=>{});
    };
    socket.onclose=()=>{
      if(caldenNotificationSocket===socket)caldenNotificationSocket=null;
      if(state.token===token&&!document.hidden){
        clearTimeout(caldenNotificationRetry);
        caldenNotificationRetry=setTimeout(connectCaldenNotifications,5000);
      }
    };
    socket.onerror=()=>socket.close();
  }catch{
    caldenNotificationSocket=null;
    clearTimeout(caldenNotificationRetry);
    caldenNotificationRetry=setTimeout(connectCaldenNotifications,10000);
  }
}
document.addEventListener("visibilitychange",()=>{
  if(document.hidden){
    clearTimeout(caldenNotificationRetry);
    caldenNotificationSocket?.close();
    caldenNotificationSocket=null;
  }else if(state.token){
    loadNotifications(true).catch(()=>{});
    connectCaldenNotifications();
  }
});
function ensureNotificationPolling(){
  if(state.me?.is_child && !(state.me.child_modules||[]).includes("notifications"))return;
  connectCaldenNotifications();
  if(state.notificationPoll)return;
  state.notificationPoll=setInterval(()=>{
    if(!state.token||document.hidden)return;
    loadNotifications(true).catch(()=>{});
  },30000);
}

function decorateNavigation(){
  // All routes use exactly one icon, one label, and optional trailing badge.
  // Module customization updates these same elements instead of appending copies.
  const icons={
    dashboard:'<path d="M3 10 12 3l9 7v11h-7v-7h-4v7H3V10Z"/>',
    calendar:'<rect x="3" y="5" width="18" height="16" rx="2"/><path d="M7 3v4m10-4v4M3 10h18"/>',
    shopping:'<path d="M3 4h2l2 11h12l2-8H6m4 13h.01M18 20h.01"/>',
    meals:'<path d="M4 3v7m4-7v7M6 3v18M3 10h6m9-7v18m0-18c-5 2-5 10 0 11"/>',
    routines:'<path d="m3 7 2 2 4-4m3 2h9M3 17l2 2 4-4m3 2h9"/>',
    rewards:'<rect x="3" y="9" width="18" height="12" rx="2"/><path d="M2 9h20M12 9v12M12 9C8 9 5 7 7 4s5 1 5 5Zm0 0c4 0 7-2 5-5s-5 1-5 5Z"/>',
    school:'<path d="m2 9 10-5 10 5-10 5L2 9Zm4 3v5c4 3 8 3 12 0v-5M22 9v8"/>',
    maintenance:'<path d="M21 7a7 7 0 0 1-9 8L5 22l-3-3 7-7A7 7 0 0 1 17 3l-4 4 4 4 4-4Z"/>',
    pets:'<path d="M12 13c-3-4-8 1-6 4 2 2 4 2 6 2s4 0 6-2c2-3-3-8-6-4ZM5 9v.01M9 5v.01M15 5v.01M19 9v.01"/>',
    bills:'<rect x="2" y="5" width="20" height="15" rx="2"/><path d="M3 9h19m-7 6h4"/>',
    agenda:'<rect x="5" y="4" width="14" height="18" rx="2"/><path d="M9 4V2h6v2M9 10h6m-6 5h6"/>',
    chores:'<path d="m3 7 2 2 4-4m3 2h9M3 17l2 2 4-4m3 2h9"/>',
    people:'<path d="M16 20v-2a4 4 0 0 0-4-4H7a4 4 0 0 0-4 4v2m6-10a4 4 0 1 0 0-8 4 4 0 0 0 0 8Zm9 4a4 4 0 0 1 3 4v2m-4-18a4 4 0 0 1 0 8"/>',
    calendars:'<rect x="7" y="4" width="12" height="16" rx="2"/><path d="M4 8v10a2 2 0 0 0 2 2M9 10h8m-8 4h6"/>',
    categories:'<path d="m4 12 8-8h7v7l-8 8L4 12Zm11-4h.01"/>',
    notifications:'<path d="M18 9a6 6 0 1 0-12 0c0 7-3 7-3 9h18c0-2-3-2-3-9Zm-8 12h4"/>',
    activity:'<path d="M12 8v5l3 2m6-3a9 9 0 1 1-3-6.7M18 2v4h4"/>',
    settings:'<path d="M12 15.5a3.5 3.5 0 1 0 0-7 3.5 3.5 0 0 0 0 7Zm7-3.5 2-1-2-3-2 .5-1.5-1L15 5H9l-.5 2.5-1.5 1L5 8l-2 3 2 1v2l-2 1 2 3 2-.5 1.5 1L9 21h6l.5-2.5 1.5-1 2 .5 2-3-2-1v-2Z"/>',
    integrations:'<path d="M8 12h8m-4-4v8M7 3v4m10-4v4M7 17v4m10-4v4M3 7h4m10 0h4M3 17h4m10 0h4"/>',
    updates:'<path d="M12 19V5m-5 5 5-5 5 5M5 21h14"/>',
    backups:'<path d="M4 7h16v13H4V7Zm3-4h10v4H7V3Zm2 9h6m-3-3v6"/>'
  };
  document.querySelectorAll(".primary-nav .nav-item").forEach(link=>{
    const badge=link.querySelector(".nav-badge");
    let glyph=link.querySelector(".nav-icon");
    if(!glyph){
      glyph=document.createElement("span");
      glyph.className="nav-icon";
      link.prepend(glyph);
    }
    if(!glyph.querySelector("svg")){
      glyph.innerHTML='<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">'+(icons[link.dataset.page]||icons.calendar)+'</svg>';
    }
    const titles=[...link.querySelectorAll(".nav-label,.family-nav-label")];
    let label=titles[0];
    if(!label){
      label=document.createElement("span");
      label.textContent=[...link.childNodes].filter(n=>n.nodeType===3).map(n=>n.textContent).join("").trim()||link.dataset.page;
    }
    label.className="nav-label";
    for(const extra of titles.slice(1))extra.remove();
    for(const node of [...link.childNodes])if(node.nodeType===3)node.remove();
    glyph.after(label);
    if(badge)link.append(badge);
  });
}

function notificationRows(){
  const now=Date.now();
  const rows=[];
  visibleEvents().forEach(event=>{
    const start=new Date(event.starts_at),end=new Date(event.ends_at);
    if(end.getTime()<now)return;
    (event.reminders||[]).forEach(reminder=>{
      const fireAt=new Date(start.getTime()-Number(reminder.minutes_before||0)*60000);
      const dueNow=fireAt.getTime()<=now&&start.getTime()>now;
      if(fireAt.getTime()<now&&!dueNow)return;
      rows.push({event,reminder,fireAt,dueNow,sortAt:dueNow?now:fireAt.getTime()});
    });
  });
  return rows.sort((a,b)=>a.sortAt-b.sortAt);
}

function renderNotifications(){
  renderNotificationInbox();
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
  host.innerHTML=rows.length?rows.slice(0,200).map(({event,reminder,fireAt,dueNow})=>{
    const system=reminder.kind==="system";
    const recipient=state.users.find(person=>person.id===reminder.recipient_user_id);
    const people=recipient?recipient.display_name:(event.assignees||[]).map(person=>person.display_name).join(", ")||"Everyone";
    return `<button type="button" class="scheduled-reminder" data-event-key="${escapeAttr(eventKey(event))}">
      <span class="scheduled-reminder-icon ${system?"system":"personal"}">${system?"M":"P"}</span>
      <span class="scheduled-reminder-when"><strong>${dueNow?"Due now":escapeHTML(formatDate(fireAt,{weekday:"short",month:"short",day:"numeric"}))}</strong><small>${dueNow?"Event "+escapeHTML(formatTime(new Date(event.starts_at))):escapeHTML(formatTime(fireAt))}</small></span>
      <span class="scheduled-reminder-main"><strong>${escapeHTML(event.title)}</strong><small>${escapeHTML(reminderLabel(reminder.minutes_before))} · ${system?"Household via Monita":"Phone reminder for "+escapeHTML(people)}</small></span>
      <span class="scheduled-reminder-calendar"><i style="--cal:${safeColor(event.calendar_color||event.color)}"></i>${escapeHTML(event.calendar_name||"Calendar")}</span>
    </button>`;
  }).join(""):'<div class="empty-state"><strong>No scheduled reminders</strong><span>Add a reminder to an event and it will appear here.</span></div>';
  host.querySelectorAll("[data-event-key]").forEach(button=>button.addEventListener("click",()=>{
    const event=findEventByKey(button.dataset.eventKey);
    if(event)openEventDetails(event);
  }));
}

function renderPeople(){
  if(state.me.role!=="admin")return;
  const devices=state.householdDevices||[];
  window.caldenChildren?.render();
  $("#people-list").innerHTML=state.users.map(u=>{
    const owned=devices.filter(d=>d.user_id===u.id);
    const deviceRows=owned.length?owned.map(d=>
      `<div class="person-device-row">📱 <strong>${escapeHTML(d.name)}</strong> · ${escapeHTML(d.platform)} · ${d.enabled?"Connected":"Disabled"} · Seen ${escapeHTML(d.last_seen_at?new Date(d.last_seen_at).toLocaleString():"Unknown")}</div>`
    ).join(""):'<p class="muted">No registered devices</p>';
    return `<article class="management-card ${u.active===false?"inactive":""}">
      <div class="management-avatar">${avatarMarkup(u)}</div>
      <div class="management-copy"><strong>${escapeHTML(u.display_name)}</strong><span>@${escapeHTML(u.username)}</span><small>${u.is_child?"Child account":roleLabel(u.role)} · ${u.active===false?"Inactive":"Active"}</small></div>
      <button class="button secondary compact edit-person" type="button" data-user-id="${escapeAttr(u.id)}">Edit</button>
      <details class="person-devices"><summary>Registered devices (${owned.length})</summary>${deviceRows}</details>
    </article>`;
  }).join("");
  $("#people-list").querySelectorAll(".edit-person").forEach(b=>b.addEventListener("click",()=>beginPersonEdit(b.dataset.userId)));
}
async function loadHouseholdDevices(){
  if(state.me?.role!=="admin")return;
  try{
    const result=await api("/api/devices/household");
    state.householdDevices=result.items||[];
    renderPeople();
  }catch(err){
    $("#person-status").textContent="Could not load household devices: "+err.message;
  }
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
  if(!["admin","planner"].includes(state.me.role))return;
  $("#calendar-list").innerHTML=state.calendars.map(c=>`<article class="management-card">
    <span class="calendar-swatch" style="--cal:${safeColor(c.color)}"></span>
    <div class="management-copy"><strong>${escapeHTML(c.name)}</strong><span>${escapeHTML(c.description||"No description")}</span><small>${c.calendar_type==="bill_pay"?"Bill Pay · ":""}${c.can_edit?"Editable":"View only"} · ${Number(c.event_count)||0} event${Number(c.event_count)===1?"":"s"}</small></div>
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
  const form=$("#calendar-form");form.reset();form.color.value="#2f6fed";form.calendar_type.value="standard";form.calendar_id.value="";state.editingCalendar=null;
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
  form.calendar_id.value=cal.id;form.name.value=cal.name;form.calendar_type.value=cal.calendar_type||"standard";form.color.value=cal.color;form.description.value=cal.description||"";
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
  if(!["admin","planner"].includes(state.me.role))return;
  $("#category-list").innerHTML=state.categories.map(cat=>`<article class="management-card">
    <span class="calendar-swatch" style="--cal:${safeColor(cat.color)}"></span>
    <div class="management-copy"><strong>${escapeHTML(cat.name)}</strong><span>${escapeHTML(cat.description||"No description")}</span><small>${Number(cat.event_count)||0} event${Number(cat.event_count)===1?"":"s"} using this category · ${escapeHTML(cat.color)}</small></div>
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

async function loadRegisteredDevices(){
  const status=$("#devices-status"),list=$("#devices-list");
  if(!status||!list)return;
  status.textContent="Loading registered devices…";
  try{
    const result=await api("/api/devices");
    const items=result.items||[];
    status.textContent=items.length?`${items.length} registered device${items.length===1?"":"s"}`:"No registered devices yet. Sign in from Calden Android to register a phone.";
    list.innerHTML=items.map(device=>`<article class="panel"><div class="form-actions">
      <div class="grow"><strong>${escapeHTML(device.name)}</strong><p class="muted">${escapeHTML(device.platform)} · Last seen ${escapeHTML(device.last_seen_at?new Date(device.last_seen_at).toLocaleString():"Unknown")}</p></div>
      <button type="button" class="button secondary compact" data-revoke-device="${escapeAttr(device.id)}">Revoke</button>
    </div></article>`).join("");
  }catch(err){status.textContent="Could not load registered devices: "+err.message}
}
// RecipeCove configuration check: no API compatibility claim or key disclosure.
async function loadRecipeCoveIntegrationStatus(){
  const status=$("#recipecove-integration-status"),badge=$("#recipecove-connection-badge");
  if(!status||!badge||state.me?.role!=="admin")return;
  status.textContent="Checking server configuration…";
  try{
    const data=await api("/api/integrations/recipecove/status");
    badge.textContent=data.enabled===false?"Disabled":data.configured?"Configured":"Not configured";
    status.textContent=data.enabled===false?
      "Meal Planning is disabled for this household. Existing RecipeCove data is unchanged.":
      data.configured?"Server configuration exists. API compatibility and connectivity are not yet verified.":
      "RecipeCove is not configured on this Calden server.";
  }catch(err){
    badge.textContent="Unavailable";
    status.textContent="Could not check RecipeCove configuration: "+err.message;
  }
}
$("#recipecove-check").addEventListener("click",loadRecipeCoveIntegrationStatus);
$("#recipecove-feature-link").addEventListener("click",()=>activateSettingsTab("optional-features"));

function activateSettingsTab(tab,writeURL=false){
  if(!["general","calendar","optional-features","account","integrations","updates","backups","devices"].includes(tab))tab="calendar";
  if(["general","optional-features","integrations","updates","backups"].includes(tab)&&state.me?.role!=="admin")tab="calendar";
  state.settingsTab=tab;
  if(writeURL&&state.currentPage==="settings")history.pushState({calden:true},"",caldenPageURL("settings"));
  $$(".settings-nav-item").forEach(button=>button.classList.toggle("active",button.dataset.settingsTab===tab));
  $$("[data-settings-pane]").forEach(pane=>pane.classList.toggle("hidden",pane.dataset.settingsPane!==tab));
  if(tab==="account")loadPrivateBusyTime();
  if(tab==="integrations"){loadMonita();setGoogleImportFile(state.googleImportFile);loadRecipeCoveIntegrationStatus()}
  if(tab==="updates")loadUpdater();
  if(tab==="backups")loadBackups();
  if(tab==="devices")loadRegisteredDevices();
}
function renderSettings(){
  const form=$("#general-settings-form");
  if(form){
    form.household_name.value=state.settings?.household_name||"";
    form.timezone.value=state.settings?.timezone||Intl.DateTimeFormat().resolvedOptions().timeZone||"UTC";
    form.week_start.value=state.settings?.week_start||"sunday";
    form.default_view.value=String(state.settings?.default_view||7);
  }
  $("#settings-tasks-enabled").checked=!!state.settings?.tasks_enabled;
  $("#settings-chores-enabled").checked=!!state.settings?.chores_enabled;
  $("#settings-shopping-enabled").checked=state.settings?.shopping_enabled!==false;
  $("#settings-meals-enabled").checked=state.settings?.meals_enabled!==false;
  $("#settings-packages-enabled").checked=state.settings?.packages_enabled===true;
  $("#settings-routines-enabled").checked=state.settings?.routines_enabled===true;
  $("#settings-rewards-enabled").checked=state.settings?.rewards_enabled===true;
  for(const kind of ["school","maintenance","pets"]){
    $("#settings-"+kind+"-enabled").checked=state.settings?.[kind+"_enabled"]===true;
  }
  $("#settings-bills-enabled").checked=state.settings?.bills_enabled!==false;
  $("#settings-agenda-enabled").checked=state.settings?.agenda_enabled!==false;
  const personalWeekStart=$("#settings-personal-week-start");
  if(personalWeekStart)personalWeekStart.value=state.me?.week_start||"default";
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
  if(state.preserveRawRecurrence&&state.editingEvent?.recurrence?.raw){
    return {...state.editingEvent.recurrence};
  }
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

function eventDetailsRow(label,value){
  if(value===null||value===undefined||value==="")return "";
  return `<div class="event-detail-row"><dt>${escapeHTML(label)}</dt><dd>${escapeHTML(value)}</dd></div>`;
}

function eventDetailsDate(event){
  const start=eventStartDate(event),end=eventEndDate(event),opts={weekday:"long",month:"long",day:"numeric",year:"numeric"};
  if(event.all_day){
    const last=addDays(end,-1);
    return sameDay(start,last)?formatDate(start,opts):formatDate(start,opts)+" – "+formatDate(last,opts);
  }
  return formatDate(start,opts)+" at "+formatTime(start)+(sameDay(start,end)?" – "+formatTime(end):" – "+formatDate(end,opts)+" at "+formatTime(end));
}

async function loadEventConfirmationPanel(event){
  const panel=$("#event-confirmation-panel");
  panel.classList.add("hidden");
  $("#event-change-reason-wrap").classList.add("hidden");
  $("#event-send-change").classList.add("hidden");
  $("#event-change-reason").value="";
  const start=event.occurrence_start||event.starts_at;
  const result=await api(`/api/events/${encodeURIComponent(event.id)}/confirmations?occurrence_start=${encodeURIComponent(start)}`);
  if(state.detailEvent!==event)return;
  event.request_confirmation=!!result.requested;
  if(!result.requested)return;
  panel.classList.remove("hidden");
  const me=String(result.current_user_id);
  const assigned=(result.assignees||[]).some(person=>String(person.user_id)===me);
  const responses=result.responses||{};
  $("#event-confirmation-people").innerHTML=(result.assignees||[]).map(person=>{
    const entry=responses[String(person.user_id)]||{status:"pending"};
    const label=entry.status==="confirmed"?"Confirmed":entry.status==="change_requested"?"Change requested":"Awaiting response";
    return `<div class="form-actions"><strong>${escapeHTML(person.display_name)}</strong><span class="muted">${escapeHTML(label)}</span></div>`;
  }).join("");
  const current=responses[me];
  $("#event-confirmation-status").textContent=assigned
    ?(current?.status==="confirmed"?"You confirmed this event.":current?.status==="change_requested"?"You requested a change.":"Your confirmation is requested.")
    :"Only assigned members can respond to this event.";
  $("#event-confirm").classList.toggle("hidden",!assigned);
  $("#event-request-change").classList.toggle("hidden",!assigned);
}

async function submitEventConfirmation(status){
  const event=state.detailEvent;
  if(!event)return;
  const start=event.occurrence_start||event.starts_at;
  const reason=status==="change_requested"?$("#event-change-reason").value.trim():"";
  if(status==="change_requested"&&!reason){
    $("#event-confirmation-status").textContent="Please describe why you need a change.";
    return;
  }
  try{
    await api(`/api/events/${encodeURIComponent(event.id)}/confirmation`,{
      method:"PUT",
      body:JSON.stringify({occurrence_start:start,status,reason})
    });
    await loadEventConfirmationPanel(event);
    await loadNotifications(false);
  }catch(err){$("#event-confirmation-status").textContent=err.message}
}

function openEventDetails(event,writeURL=true){
  if(!event)return;
  state.detailEvent=event;
  if(writeURL)history.pushState({calden:true},"",caldenEventURL(event));
  const bill=isBillCalendar(event.calendar_id);
  $("#event-details-eyebrow").textContent=bill?"Bill details":"Event details";
  $("#event-details-title").textContent=event.title||"Untitled event";
  $("#event-details-link").textContent="Copy link";
  const rows=[
    eventDetailsRow("Calendar",event.calendar_name||"Calendar"),
    eventDetailsRow("When",eventDetailsDate(event)),
    eventDetailsRow("Category",event.category_name),
    eventDetailsRow("Assigned to",(event.assignees||[]).map(user=>user.display_name).join(", ")||"Not assigned"),
    eventDetailsRow("Location",event.location),
    eventDetailsRow("Recurring",event.is_recurring?"Yes":null)
  ];
  if(bill){
    rows.push(eventDetailsRow("Expected amount",event.bill_amount==null?"Not set":money(event.bill_amount)+(event.bill_amount_is_estimate?" (estimated)":"")));
    rows.push(eventDetailsRow("Assigned payer",event.bill_payer?.display_name||"Not assigned"));
    rows.push(eventDetailsRow("Status",billStatusLabel(billPaymentStatus(event))));
    rows.push(eventDetailsRow("Allocated",money(billAllocatedAmount(event))));
    rows.push(eventDetailsRow("Paid",money(billPaidAmount(event))));
    rows.push(eventDetailsRow("Remaining",billRemaining(event)===null?"Not set":money(billRemaining(event))));
    billPayments(event).forEach((payment,index)=>rows.push(eventDetailsRow("Payment "+(index+1),money(payment.amount_paid)+" · "+(payment.paid_by?.display_name||"Unknown payer")+" · paid "+(payment.paid_on||"date unknown")+(payment.cleared_on?" · cleared "+payment.cleared_on:" · not cleared"))));
  }
  $("#event-details-fields").innerHTML=rows.join("");
  loadEventConfirmationPanel(event).catch(err=>{
    $("#event-confirmation-status").textContent=err.message;
  });
  $("#event-details-notes").classList.toggle("hidden",!event.notes);
  $("#event-details-notes-text").textContent=event.notes||"";
  const editable=!!state.calendars.find(calendar=>calendar.id===event.calendar_id&&calendar.can_edit);
  $("#event-details-edit").classList.toggle("hidden",!editable);
  $("#event-details-bill").classList.toggle("hidden",!bill||!canUpdateBill(event));
  if(!$("#event-details-dialog").open)$("#event-details-dialog").showModal();
}

function closeEventDetails(){
  state.suppressDetailCloseRoute=true;
  $("#event-details-dialog").close();
  state.detailEvent=null;
  if(location.pathname.startsWith("/events/")||location.pathname.startsWith("/bills/"))
    history.pushState({calden:true},"",caldenPageURL(state.currentPage));
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
    ${personal?`<select class="reminder-recipient" aria-label="Reminder recipient"><option value="">All event assignees</option>${state.users.filter(user=>user.active!==false).map(user=>`<option value="${escapeAttr(user.id)}" ${reminder.recipient_user_id===user.id?"selected":""}>${escapeHTML(user.display_name)}</option>`).join("")}</select>`:`<input class="reminder-destination" maxlength="200" placeholder="Monita channel (optional)" value="${escapeAttr(reminder.destination||"")}">`}
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
    reminders.push({kind:"personal",provider:"android",minutes_before:Number(row.querySelector(".reminder-minutes").value),destination:"",recipient_user_id:row.querySelector(".reminder-recipient")?.value||null});
  });
  $("#system-reminders-list").querySelectorAll(".reminder-row").forEach(row=>{
    reminders.push({kind:"system",provider:"monita",minutes_before:Number(row.querySelector(".reminder-minutes").value),destination:row.querySelector(".reminder-destination")?.value.trim()||""});
  });
  return reminders;
}

function updateBillEventUI(){
  const form=$("#event-form"),bill=isBillCalendar(form.calendar_id.value);
  $("#bill-event-details")?.classList.toggle("hidden",!bill);
  if(!bill){
    form.bill_amount.value="";
    form.bill_amount_is_estimate.checked=false;
    form.bill_payer_user_id.value="";
  }
}
function openBillEvent(){
  const cal=state.calendars.find(item=>item.calendar_type==="bill_pay"&&item.can_edit);
  if(!cal){alert("You do not have a Bill Pay calendar you can edit.");return}
  openEvent();
  const form=$("#event-form");form.calendar_id.value=cal.id;updateBillEventUI();
}

function openEvent(existing=null,dateHint=null,scope="series"){
  if(!state.calendars.some(c=>c.can_edit)){
    if(["admin","planner"].includes(state.me.role)){navigate("calendars");return}
    alert("You do not have a calendar you can add events to yet.");return;
  }
  const form=$("#event-form");form.reset();form.request_confirmation.checked=!!existing?.request_confirmation;state.editingEvent=existing;state.editingScope=scope;state.preserveRawRecurrence=false;
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
    const sourceStartsAt=occurrenceScope?existing.starts_at:(existing.series_starts_at||existing.starts_at);
    const sourceEndsAt=occurrenceScope?existing.ends_at:(existing.series_ends_at||existing.ends_at);
    form.starts_at.value=localInput(sourceAllDay?calendarDate(sourceStartsAt):new Date(sourceStartsAt));
    form.ends_at.value=localInput(sourceAllDay?calendarDate(sourceEndsAt):new Date(sourceEndsAt));
    form.all_day.checked=!!sourceAllDay;form.location.value=sourceLocation||"";form.notes.value=sourceNotes||"";
    const sourceBillAmount=occurrenceScope?existing.bill_amount:seriesValue(existing,"bill_amount",existing.bill_amount);
    const sourceBillEstimate=occurrenceScope?existing.bill_amount_is_estimate:seriesValue(existing,"bill_amount_is_estimate",existing.bill_amount_is_estimate);
    const sourceBillPayer=occurrenceScope?existing.bill_payer:seriesValue(existing,"bill_payer",existing.bill_payer);
    form.bill_amount.value=sourceBillAmount===null||sourceBillAmount===undefined?"":String(sourceBillAmount);
    form.bill_amount_is_estimate.checked=!!sourceBillEstimate;
    form.bill_payer_user_id.value=sourceBillPayer?.id||"";
    const recurrence=occurrenceScope?null:(existing.recurrence||null);
    state.preserveRawRecurrence=!!recurrence?.raw;
    $("#advanced-recurrence-note")?.classList.toggle("hidden",!state.preserveRawRecurrence);
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
    $("#repeat-editor").classList.remove("hidden");$("#series-scope-note").classList.add("hidden");$("#advanced-recurrence-note")?.classList.add("hidden");
    const start=eventStartFromHint(dateHint);
    const end=new Date(start.getTime()+state.defaultDuration*60000);
    form.starts_at.value=localInput(start);form.ends_at.value=localInput(end);
    const editable=state.calendars.filter(cal=>cal.can_edit);
    const preferred=editable.find(cal=>cal.id===state.defaultCalendar)||editable[0];
    if(preferred)form.calendar_id.value=preferred.id;
    form.repeat_frequency.value="";form.repeat_interval.value="1";form.repeat_end_type.value="never";form.repeat_count.value="10";
    form.bill_amount.value="";form.bill_amount_is_estimate.checked=false;form.bill_payer_user_id.value="";
    [...form.querySelectorAll('input[name="repeat_weekday"]')].forEach(i=>i.checked=false);
    renderReminderEditor([]);
  }
  updateBillEventUI();
  updateRepeatUI();
  $("#event-error").textContent="";refreshEventDateReview();$("#event-dialog").showModal();
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
    $("#restore-status").textContent="A validated restore is staged and ready. Restart Calden to apply it.";
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
    <div class="saved-backup-actions"><button class="button secondary compact backup-download" data-name="${escapeAttr(b.name)}" type="button">Download</button><button class="button danger compact backup-delete" data-name="${escapeAttr(b.name)}" type="button" aria-label="Delete backup ${escapeAttr(b.name)}">Delete</button></div>
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
  return ({event:"event",calendar:"calendar",category:"category",user:"person",settings:"household settings",integration:"integration",update:"Calden"})[type]||type;
}
async function loadActivity(){
  if(state.me.role!=="admin")return;
  const host=$("#activity-list");host.innerHTML='<div class="empty-state"><strong>Loading activity…</strong></div>';
  const filter=$("#activity-filter").value;
  try{
    const items=await api("/api/activity?limit=250"+(filter?"&entity_type="+encodeURIComponent(filter):""));
    host.innerHTML=items.length?items.map(activityRow).join(""):'<div class="empty-state"><strong>No activity yet</strong><span>Changes made in Calden will appear here.</span></div>';
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
    const options=form.channel_id;
    if(options){
      options.innerHTML='<option value="0">Select a channel</option>';
      if(m.channel_id){const option=document.createElement("option");option.value=String(m.channel_id);option.textContent="Saved channel #"+m.channel_id;option.selected=true;options.append(option)}
      options.dataset.selected=String(m.channel_id||0);
    }
  }catch(err){$("#monita-status").textContent=err.message}
}


function setGoogleImportFile(file){
  state.googleImportFile=file||null;
  state.googleImportPreview=null;
  const summary=$("#google-import-file-summary"),review=$("#review-google-calendar"),importButton=$("#import-google-calendar"),status=$("#google-import-status");
  const mapping=$("#google-import-mapping"),results=$("#google-import-results");
  if(mapping){mapping.classList.add("hidden");mapping.innerHTML=""}
  if(results){results.classList.add("hidden");results.innerHTML=""}
  if(importButton){importButton.classList.add("hidden");importButton.disabled=true}
  if(!summary||!review)return;
  if(!file){
    summary.classList.add("hidden");summary.innerHTML="";review.disabled=true;
    if(status)status.textContent="";
    return;
  }
  const valid=/\.(zip|ics)$/i.test(file.name||"");
  summary.classList.remove("hidden");
  summary.innerHTML=`<div><strong>${escapeHTML(file.name||"Google Calendar export")}</strong><span>${humanSize(file.size||0)}</span></div><button id="clear-google-import-file" class="text-button" type="button">Clear</button>`;
  review.disabled=!valid;
  if(!valid)status.textContent="Choose the .zip file exported by Google Calendar, or an .ics file.";
  else status.textContent="Review the export before importing. Nothing will be created yet.";
  $("#clear-google-import-file")?.addEventListener("click",()=>{
    const input=$("#google-calendar-export");if(input)input.value="";
    setGoogleImportFile(null);
  });
}

function googleMappingOptions(item){
  const suggested=item.suggested_calendar_id||"";
  const options=[
    `<option value="__skip__" ${!suggested?"selected":""}>Skip this Google calendar</option>`,
    `<option value="__create__">Create a new Calden calendar</option>`
  ];
  state.calendars.forEach(cal=>{
    const selected=cal.id===suggested?"selected":"";
    const type=cal.calendar_type==="bill_pay"?" · Bill Pay":"";
    options.push(`<option value="${cal.id}" ${selected}>${escapeHTML(cal.name)}${type}</option>`);
  });
  return options.join("");
}

function renderGoogleImportMapping(body){
  const host=$("#google-import-mapping"),button=$("#import-google-calendar");
  const items=body?.calendars||[];
  state.googleImportPreview=items;
  host.classList.remove("hidden");
  if(!items.length){
    host.innerHTML='<div class="empty-state"><strong>No calendars found</strong></div>';
    button.classList.add("hidden");button.disabled=true;return;
  }
  const duplicateSuppressed=Number(body?.duplicate_events_suppressed)||0;
  host.innerHTML=`<div class="google-mapping-head"><div><strong>Review calendar mapping</strong><span>Nothing is imported until you confirm these choices. ${duplicateSuppressed?duplicateSuppressed+" stale duplicate Google event"+(duplicateSuppressed===1?" was":"s were")+" removed from the export before mapping.":""}</span></div><span>${items.length} Google calendar${items.length===1?"":"s"}</span></div>
    <div class="google-mapping-list">${items.map((item,index)=>{
      const prior=item.previous_auto_created?" · previous import created a duplicate calendar":"";
      const suppressed=Number(item.duplicate_events_suppressed)||0;
      const duplicateNote=suppressed?" · "+suppressed+" stale duplicate"+(suppressed===1?"":"s")+" ignored":"";
      return `<article class="google-mapping-row">
        <div class="google-mapping-source"><strong>${escapeHTML(item.name)}</strong><small>${Number(item.event_count)||0} event${Number(item.event_count)===1?"":"s"} to import${escapeHTML(duplicateNote)}${escapeHTML(prior)}</small></div>
        <label>Import into<select class="google-map-select" data-google-map-index="${index}">${googleMappingOptions(item)}</select></label>
        <div class="google-match-reason ${item.match_score>=80?"match-good":""}"><strong>${item.match_score>=80?"Suggested match":"Review required"}</strong><span>${escapeHTML(item.match_reason||"Choose where this calendar belongs.")}</span></div>
      </article>`;
    }).join("")}</div>`;
  button.classList.remove("hidden");button.disabled=false;
}

function collectGoogleMapping(){
  const mapping={};
  (state.googleImportPreview||[]).forEach((item,index)=>{
    const select=$(`[data-google-map-index="${index}"]`);
    mapping[item.external_id]=select?.value||"__skip__";
  });
  return mapping;
}

function renderGoogleImportResults(body){
  const host=$("#google-import-results");if(!host)return;
  const calendars=body?.calendars||[],warnings=body?.warnings||[];
  host.classList.remove("hidden");
  host.innerHTML=`<div class="google-import-summary">
    <span>${Number(body?.calendar_count)||calendars.length} imported calendars</span>
    <span>${Number(body?.created)||0} new events</span>
    <span>${Number(body?.updated)||0} updated</span>
    <span>${Number(body?.skipped_calendars)||0} skipped calendars</span>
    <span>${Number(body?.cleaned_calendars)||0} old duplicate calendars cleaned up</span>
    <span>${Number(body?.duplicate_events_suppressed)||0} stale Google event copies suppressed</span>
    <span>${Number(body?.duplicate_events_reconciled)||0} existing duplicate events repaired</span>
  </div>`+calendars.map(item=>`<article class="google-import-calendar">
    <div><strong>${escapeHTML(item.name||"Imported calendar")}</strong><small>${Number(item.created)||0} new · ${Number(item.updated)||0} updated${item.skipped?" · "+Number(item.skipped)+" skipped":""}</small></div>
    <span>${item.created_calendar?"Created calendar":"Mapped to existing"}</span>
  </article>`).join("")+
  (warnings.length?`<div class="google-import-warnings"><strong>Kept for safety</strong>${warnings.map(warning=>`<span>${escapeHTML(warning)}</span>`).join("")}</div>`:"");
}

async function loadUpdater(){
  if(state.me?.role!=="admin")return;
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
  // A background status poll must not overwrite unsaved administrator choices.
  const updateChannel=$("#update-channel"),autoCheck=$("#update-auto-check");
  if(!state.updatePrefsDirty){
    updateChannel.value=info.preferences?.channel||"stable";
    autoCheck.checked=info.preferences?.auto_check!==false;
  }
  const status=info.status||{state:"idle",progress:0,activity:[]};
  const active=["backup","preparing","downloading","verifying","installing","restarting"].includes(status.state);
  // Progress belongs to an active installation only; completed steps are optional history.
  $("#update-progress-wrap").classList.toggle("hidden",!active);
  $("#update-progress").style.width=`${clamp(Number(status.progress)||0,0,100)}%`;
  $("#update-progress-label").textContent=`${Number(status.progress)||0}%`;
  $("#update-state").className=`update-state state-${status.state||"idle"}`;
  if(active)$("#update-state").textContent=status.message||status.step||"Installing update…";
  else if(status.state==="completed")$("#update-state").textContent="Update completed successfully. Calden "+(info.current_version||"")+" is installed.";
  else if(status.state==="failed")$("#update-state").textContent="Update failed: "+(status.message||"Review the activity details below.");
  else if(info.available)$("#update-state").textContent=`Calden ${latest.version} is available.`;
  else $("#update-state").textContent="Calden is up to date for this channel.";
  $("#apply-update").classList.toggle("hidden",!info.available||active);
  $("#apply-update").dataset.version=latest?.version||"";
  $("#rollback-update").classList.toggle("hidden",!status.rollback_ready||active);
  $("#update-release-notes").innerHTML=latest?`<h3>${escapeHTML(latest.name||"Release "+latest.version)}</h3><p class="release-meta">${latest.published_at?escapeHTML(formatDate(new Date(latest.published_at),{month:"long",day:"numeric",year:"numeric"})):""}</p><div>${escapeHTML(latest.notes||"No release notes were provided.").replace(/\n/g,"<br>")}</div>`:'<p class="muted">No eligible release was found for this channel.</p>';
  const historyDetails=$("#update-history-details");
  historyDetails.classList.toggle("hidden",!(status.activity||[]).length);
  if(active||status.state==="failed")historyDetails.open=true;
  else historyDetails.open=false;
  $("#update-activity").innerHTML=(status.activity||[]).slice().reverse().map(a=>`<div><span>${a.timestamp&&!Number.isNaN(new Date(a.timestamp).getTime())?escapeHTML(formatTime(new Date(a.timestamp))):"—"}</span><strong>${escapeHTML(a.message)}</strong></div>`).join("");
}
function startUpdatePolling(){
  if(state.updatePoll!==null)clearTimeout(state.updatePoll);
  const poll=async()=>{
    // Keep at most one request in flight and avoid racing overlapping polls.
    state.updatePoll=null;
    if(state.me?.role!=="admin")return;
    try{
      const status=await api("/api/system/update/status");
      if(state.updateInfo){state.updateInfo.status=status;renderUpdater(state.updateInfo)}
      if(!["backup","preparing","downloading","verifying","installing","restarting"].includes(status.state)){
        state.updatePoll=setTimeout(()=>{checkCalDenClientVersion();loadUpdater()},600);
        return;
      }
    }catch(err){
      $("#update-state").textContent="Update status temporarily unavailable: "+err.message;
    }
    state.updatePoll=setTimeout(poll,1500);
  };
  state.updatePoll=setTimeout(poll,1000);
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
$$("[data-page]").forEach(b=>{
  // Keep static href values available for open-in-new-tab and copying links.
  b.addEventListener("click",event=>{
    if(event.defaultPrevented||event.button!==0||event.metaKey||event.ctrlKey||event.shiftKey||event.altKey)return;
    event.preventDefault();navigate(b.dataset.page);
  });
});
$("#mobile-menu").addEventListener("click",()=>$("#sidebar").classList.toggle("open"));
$("#event-details-link").addEventListener("click",async()=>{
  const event=state.detailEvent;if(!event)return;
  const url=new URL(caldenEventURL(event),location.origin).href;
  try{await navigator.clipboard.writeText(url);$("#event-details-link").textContent="Copied link"}
  catch{window.prompt("Copy event link",url)}
});
$("#event-details-close").addEventListener("click",closeEventDetails);
$("#event-details-done").addEventListener("click",closeEventDetails);
$("#event-details-dialog").addEventListener("close",()=>{
  if(state.suppressDetailCloseRoute){state.suppressDetailCloseRoute=false;return}
  state.detailEvent=null;
  if(location.pathname.startsWith("/events/")||location.pathname.startsWith("/bills/"))
    history.replaceState({calden:true},"",caldenPageURL(state.currentPage));
});
$("#event-details-edit").addEventListener("click",()=>{const selected=state.detailEvent;if(!selected)return;closeEventDetails();requestEventEdit(selected)});
$("#event-details-bill").addEventListener("click",()=>{const selected=state.detailEvent;if(!selected)return;closeEventDetails();openBillPayment(selected)});
$("#new-event").addEventListener("click",()=>openEvent());
$("#nav-add").addEventListener("click",()=>openEvent());
$("#add-bill")?.addEventListener("click",openBillEvent);
$("#bill-payment-cancel")?.addEventListener("click",()=>{$("#bill-payment-dialog").close();state.billPaymentEvent=null;state.billPaymentEditingId=null});
$("#bill-payment-form")?.addEventListener("submit",async e=>{e.preventDefault();await saveBillPaymentEntry()});
$("#bill-payment-reset")?.addEventListener("click",()=>resetBillPaymentEntry());
$("#bill-payment-no-balance")?.addEventListener("click",toggleBillNoBalance);
$("#bill-allocation-save")?.addEventListener("click",saveBillAllocation);
$("#bill-allocation-clear")?.addEventListener("click",clearBillAllocation);
$("#bill-prev")?.addEventListener("click",async()=>{state.billMonth=new Date(state.billMonth.getFullYear(),state.billMonth.getMonth()-1,1);history.pushState({calden:true},"",caldenPageURL("bills"));await loadBillMonth()});
$("#bill-next")?.addEventListener("click",async()=>{state.billMonth=new Date(state.billMonth.getFullYear(),state.billMonth.getMonth()+1,1);history.pushState({calden:true},"",caldenPageURL("bills"));await loadBillMonth()});
$("#bill-current")?.addEventListener("click",async()=>{const now=new Date();state.billMonth=new Date(now.getFullYear(),now.getMonth(),1);history.pushState({calden:true},"",caldenPageURL("bills"));await loadBillMonth()});
$("#event-form").elements.namedItem("calendar_id").addEventListener("change",updateBillEventUI);
$("#calendar-strip").addEventListener("click",e=>{
  const b=e.target.closest("[data-calendar-id]");if(!b)return;
  const id=b.dataset.calendarId;if(state.hiddenCalendars.has(id))state.hiddenCalendars.delete(id);else state.hiddenCalendars.add(id);
  localStorage.setItem("calden_hidden_calendars",JSON.stringify([...state.hiddenCalendars]));renderCalendar();renderAgenda();renderSettings();
});
$$(".view-switcher button").forEach(b=>b.addEventListener("click",async()=>{
  state.viewDays=Number(b.dataset.days);localStorage.setItem("calden_view_days",String(state.viewDays));
  history.pushState({calden:true},"",caldenPageURL("calendar"));
  await loadEvents();renderCalendar();renderAgenda();
}));
$("#calendar-prev").addEventListener("click",async()=>{state.anchorDate=addDays(state.anchorDate,-state.viewDays);history.pushState({calden:true},"",caldenPageURL("calendar"));await loadEvents();renderCalendar();renderAgenda()});
$("#calendar-next").addEventListener("click",async()=>{state.anchorDate=addDays(state.anchorDate,state.viewDays);history.pushState({calden:true},"",caldenPageURL("calendar"));await loadEvents();renderCalendar();renderAgenda()});
$("#calendar-today").addEventListener("click",async()=>{state.anchorDate=startOfDay(new Date());history.pushState({calden:true},"",caldenPageURL("calendar"));await loadEvents();renderCalendar();renderAgenda()});
$("#calendar-category-filter").addEventListener("change",e=>{state.filters.category=e.target.value;renderCalendar();renderAgenda()});
$("#calendar-person-filter").addEventListener("change",e=>{state.filters.person=e.target.value;renderCalendar();renderAgenda()});
$("#calendar-search").addEventListener("input",e=>{state.filters.query=e.target.value.trim();renderCalendar();renderAgenda()});
$("#clear-calendar-filters").addEventListener("click",()=>{
  state.filters={category:"",person:"",query:""};renderCalendar();renderAgenda();
});
$("#agenda-search").addEventListener("input",renderAgenda);
$("#agenda-kind").addEventListener("change",renderAgenda);
$("#agenda-calendar").addEventListener("change",renderAgenda);
$("#agenda-range").addEventListener("change",renderAgenda);
$("#notification-kind-filter").addEventListener("change",renderNotifications);
$("#refresh-notifications").addEventListener("click",async()=>{await Promise.all([loadEvents(),loadNotifications(false)]);renderNotifications()});
$("#mark-notifications-read").addEventListener("click",async()=>{
  try{
    await api("/api/notifications/read-all",{method:"PUT"});
    state.notifications.forEach(item=>{if(!item.read_at)item.read_at=new Date().toISOString()});
    state.unreadNotifications=0;
    renderNotificationInbox();
  }catch{}
});
$("#enable-browser-notifications").addEventListener("click",async()=>{
  if(!("Notification" in window))return;
  try{await Notification.requestPermission()}catch{}
  browserNotificationButton();
});
$$("#event-dialog [data-close-event]").forEach(b=>b.addEventListener("click",()=>$("#event-dialog").close()));
$("#add-personal-reminder").addEventListener("click",()=>addReminderRow("personal"));
$("#create-scheduled-reminder").addEventListener("click",()=>{openEvent();addReminderRow("personal")});
$("#add-system-reminder").addEventListener("click",()=>addReminderRow("system"));
function markRecurrenceEdited(){
  if(!state.preserveRawRecurrence)return;
  state.preserveRawRecurrence=false;
  $("#advanced-recurrence-note")?.classList.add("hidden");
}
$("#event-form").repeat_frequency.addEventListener("change",()=>{markRecurrenceEdited();updateRepeatUI()});
$("#event-form").repeat_interval.addEventListener("input",()=>{markRecurrenceEdited();updateRepeatUI()});
$("#event-form").repeat_end_type.addEventListener("change",()=>{markRecurrenceEdited();updateRepeatUI()});
$("#event-form").repeat_until.addEventListener("change",markRecurrenceEdited);
$("#event-form").repeat_count.addEventListener("input",markRecurrenceEdited);
$("#event-form").querySelectorAll('input[name="repeat_weekday"]').forEach(input=>input.addEventListener("change",markRecurrenceEdited));
$("#event-form").starts_at.addEventListener("change",()=>{if($("#event-form").repeat_frequency.value==="weekly")updateRepeatUI()});

function refreshEventDateReview(){
  const form=$("#event-form"),display=$("#event-date-review");
  if(!display)return;
  const from=form.starts_at.value,to=form.ends_at.value;
  if(!from||!to){display.textContent="Choose both the start and end date.";return}
  const start=new Date(from),end=new Date(to);
  if(!Number.isFinite(start.getTime())||!Number.isFinite(end.getTime())){display.textContent="Check the appointment date and time.";return}
  const human=value=>value.toLocaleString(undefined,{weekday:"short",month:"long",day:"numeric",year:"numeric",hour:"numeric",minute:"2-digit"});
  display.textContent="Please verify: "+human(start)+" through "+human(end);
}
$("#event-form").starts_at.addEventListener("input",refreshEventDateReview);
$("#event-form").ends_at.addEventListener("input",refreshEventDateReview);
$("#event-form").ends_at.addEventListener("change",refreshEventDateReview);

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
    state.editingEvent=null;state.editingScope="series";state.preserveRawRecurrence=false;
    state.calendars=await api("/api/calendars");
    await Promise.all([loadEvents(),loadNotifications(false)]);
    renderCalendars();renderBillNavigation();renderEventControls();renderCalendar();renderAgenda();renderNotifications();
    if(state.currentPage==="bills")await loadBillMonth();else renderBills();
  }catch(err){
    if(!$("#event-dialog").open)$("#event-dialog").showModal();
    $("#event-error").textContent=err.message;
  }
});
$("#event-form").addEventListener("submit",async e=>{
  e.preventDefault();const form=e.currentTarget,fd=new FormData(form);$("#event-error").textContent="";
  const reminders=collectReminders();
  const billActive=isBillCalendar(fd.get("calendar_id"));
  const billRaw=String(fd.get("bill_amount")||"").trim();
  const startAt=new Date(fd.get("starts_at")),endAt=new Date(fd.get("ends_at"));
  if(!Number.isFinite(startAt.getTime())||!Number.isFinite(endAt.getTime())||endAt<=startAt){
    $("#event-error").textContent="Check the start and end date before saving.";return;
  }
  const payload={title:fd.get("title"),calendar_id:fd.get("calendar_id"),category_id:fd.get("category_id")||null,starts_at:startAt.toISOString(),ends_at:endAt.toISOString(),all_day:form.all_day.checked,location:fd.get("location"),notes:fd.get("notes"),assignee_ids:fd.getAll("assignee"),request_confirmation:form.request_confirmation.checked,reminders,recurrence:recurrencePayload(form),bill_amount:billActive&&billRaw!==""?Number(billRaw):null,bill_amount_is_estimate:billActive&&form.bill_amount_is_estimate.checked,bill_payer_user_id:billActive&&form.bill_payer_user_id.value?form.bill_payer_user_id.value:null};
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
    if(!billActive && !payload.all_day) {
      const preview=await api("/api/scheduling/conflicts",{
        method:"POST",
        body:JSON.stringify({
          calendar_id:payload.calendar_id,
          exclude_event_id:state.editingEvent?.id||null,
          starts_at:payload.starts_at,ends_at:payload.ends_at,
          assignee_ids:payload.assignee_ids,
          recurrence:payload.recurrence
        })
      });
      if(preview.conflicts?.length) {
        const count=preview.conflicts.length;
        if(!confirm(count+" scheduling conflict(s) were found. Private busy times do not show their details. Save this appointment anyway?"))return;
      }
    }
    const saved=await api(target,{method,body:JSON.stringify(body)});
    if(method!=="DELETE"){
      if(saved?.verification!=="confirmed"){
        $("#event-error").textContent="The event was saved, but its stored date could not be verified. Refresh the calendar before attempting to save it again.";return;
      }
      const storedStart=Date.parse(saved.starts_at),storedEnd=Date.parse(saved.ends_at);
      if(storedStart!==startAt.getTime()||storedEnd!==endAt.getTime()){
        $("#event-error").textContent="WARNING: Event was saved with a different date than entered. Please review the event in Calendar; do not submit it twice.";return;
      }
    }
    // A saved January appointment must immediately open January in Calendar.
    const savedDate=payload.all_day?calendarDate(String(fd.get("starts_at")).slice(0,10)):new Date(payload.starts_at);
    if(Number.isFinite(savedDate.getTime())){
      // Bill Pay is a separate workflow: saving a bill must not navigate
      // away from the bill list, which would also hide the saved result.
      if(billActive){
        state.billMonth=new Date(savedDate.getFullYear(),savedDate.getMonth(),1);
      }
      if(state.currentPage!=="bills"){
        state.anchorDate=startOfDay(savedDate);
        navigate("calendar",false);
      }
    }
    $("#event-dialog").close();state.editingEvent=null;state.editingScope="series";state.preserveRawRecurrence=false;
    state.calendars=await api("/api/calendars");
    await Promise.all([loadEvents(),loadNotifications(false)]);
    renderCalendars();renderBillNavigation();renderEventControls();renderCalendar();renderAgenda();renderNotifications();
    if(state.currentPage==="bills")await loadBillMonth();else renderBills();
  }
  catch(err){$("#event-error").textContent=err.message}
});

$("#person-form").addEventListener("submit",async e=>{
  e.preventDefault();const form=e.currentTarget;$("#person-error").textContent="";$("#person-status").textContent="";
  const editing=state.editingUser;
  const avatarFile=$("#person-avatar-file").files?.[0];
  if(avatarFile&&avatarFile.size>maxAvatarUploadBytes){
    $("#person-error").textContent="Profile image must be 10 MB or smaller.";
    return;
  }
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
    $("#refresh-devices").addEventListener("click",loadRegisteredDevices);
$("#devices-list").addEventListener("click",async event=>{
  const button=event.target.closest("[data-revoke-device]");
  if(!button)return;
  if(!confirm("Revoke this device? It will need to register again before receiving future notifications."))return;
  button.disabled=true;
  try{
    await api("/api/devices/"+encodeURIComponent(button.dataset.revokeDevice),{method:"DELETE"});
    await loadRegisteredDevices();
  }catch(err){
    $("#devices-status").textContent=err.message;
    button.disabled=false;
  }
});

$("#event-confirm").addEventListener("click",()=>submitEventConfirmation("confirmed"));
$("#event-request-change").addEventListener("click",()=>{
  $("#event-change-reason-wrap").classList.remove("hidden");
  $("#event-send-change").classList.remove("hidden");
});
$("#event-send-change").addEventListener("click",()=>submitEventConfirmation("change_requested"));



$("#person-avatar-file").addEventListener("change",e=>{
  if(!e.target.files?.[0])return;
  previewAvatarFile(e.target,$("#person-avatar-preview"),$("#person-error"));
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
  const payload={name:fd.get("name"),color:fd.get("color"),icon:fd.get("calendar_type")==="bill_pay"?"receipt":"calendar",description:fd.get("description"),calendar_type:fd.get("calendar_type")||"standard"};
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
    state.calendars=await api("/api/calendars");resetCalendarForm();renderCalendars();renderBillNavigation();renderBills();renderCalendar();renderEventControls();
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

function calendarDeletePhrase(calendar){
  return "DELETE "+calendar.name;
}

function openCalendarDeleteDialog(calendar,eventCount){
  const dialog=$("#calendar-delete-dialog");
  const phrase=calendarDeletePhrase(calendar);
  $("#calendar-delete-name").textContent=calendar.name;
  $("#calendar-delete-event-count").textContent=`${eventCount} event${eventCount===1?"":"s"} will be permanently deleted.`;
  $("#calendar-delete-phrase").textContent=phrase;
  $("#calendar-delete-confirmation").value="";
  $("#calendar-delete-error").textContent="";
  $("#calendar-delete-confirm").disabled=true;
  dialog.showModal();
  $("#calendar-delete-confirmation").focus();
}

async function finishCalendarDelete(calendar,options={}){
  await api("/api/calendars/"+calendar.id,{method:"DELETE",...(options.body?{body:JSON.stringify(options.body)}:{})});
  state.hiddenCalendars.delete(calendar.id);
  localStorage.setItem("calden_hidden_calendars",JSON.stringify([...state.hiddenCalendars]));
  if(state.defaultCalendar===calendar.id){
    state.defaultCalendar="";
    localStorage.removeItem("calden_default_calendar");
  }
  [state.calendars,state.categories]=await Promise.all([api("/api/calendars"),api("/api/categories")]);
  await loadEvents();
  resetCalendarForm();
  renderCalendars();renderBillNavigation();renderBills();renderEventControls();renderCalendar();renderAgenda();renderNotifications();renderSettings();
}

$("#delete-calendar").addEventListener("click",async()=>{
  const calendar=state.editingCalendar;
  if(!calendar)return;
  const knownCount=Number(calendar.event_count)||0;
  if(knownCount>0){
    openCalendarDeleteDialog(calendar,knownCount);
    return;
  }
  if(!confirm(`Delete "${calendar.name}"? This permanently deletes the calendar.`))return;
  try{
    await finishCalendarDelete(calendar);
  }catch(err){
    if(err.status===409&&err.body?.requires_typed_confirmation){
      openCalendarDeleteDialog(calendar,Number(err.body.event_count)||1);
      return;
    }
    $("#calendar-error").textContent=err.message;
  }
});

$("#calendar-delete-confirmation").addEventListener("input",()=>{
  const calendar=state.editingCalendar;
  $("#calendar-delete-confirm").disabled=!calendar||$("#calendar-delete-confirmation").value!==calendarDeletePhrase(calendar);
});
$("#calendar-delete-cancel").addEventListener("click",()=>$("#calendar-delete-dialog").close());
$("#calendar-delete-form").addEventListener("submit",async e=>{
  e.preventDefault();
  const calendar=state.editingCalendar;
  if(!calendar)return;
  const confirmation=$("#calendar-delete-confirmation").value;
  if(confirmation!==calendarDeletePhrase(calendar))return;
  const button=$("#calendar-delete-confirm");
  button.disabled=true;
  button.textContent="Deleting…";
  $("#calendar-delete-error").textContent="";
  try{
    await finishCalendarDelete(calendar,{body:{force:true,confirmation}});
    $("#calendar-delete-dialog").close();
  }catch(err){
    $("#calendar-delete-error").textContent=err.message;
  }finally{
    button.textContent="Delete calendar and events";
    button.disabled=!state.editingCalendar||$("#calendar-delete-confirmation").value!==calendarDeletePhrase(state.editingCalendar);
  }
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
  if(!state.editingCategory)return;
  const category=state.editingCategory;
  const count=Number(category.event_count)||0;
  const impact=count?` ${count} event${count===1?"":"s"} will become uncategorized; the events themselves will not be deleted.`:"";
  if(!confirm(`Permanently delete "${category.name}"?${impact}`))return;
  try{
    await api("/api/categories/"+category.id,{method:"DELETE"});
    if(state.filters.category===category.id)state.filters.category="";
    state.categories=await api("/api/categories");
    resetCategoryForm();renderCategories();renderEventControls();await loadEvents();renderCalendar();renderAgenda();
  }catch(err){$("#category-error").textContent=err.message}
});

$$(".settings-nav-item").forEach(button=>button.addEventListener("click",()=>activateSettingsTab(button.dataset.settingsTab,true)));
$("#display-settings-form").addEventListener("submit",async e=>{
  e.preventDefault();
  state.defaultCalendar=$("#settings-default-calendar").value;
  state.defaultDuration=Number($("#settings-default-duration").value)||60;
  state.scrollNow=$("#settings-scroll-now").checked;
  const weekStart=$("#settings-personal-week-start").value;
  try {
    await api("/api/me/week-start",{method:"PUT",body:JSON.stringify({week_start:weekStart})});
    state.me.week_start=weekStart==="default"?null:weekStart;
  } catch(err) {
    $("#display-settings-status").textContent=err.message;
    return;
  }
  localStorage.setItem("calden_default_calendar",state.defaultCalendar);
  localStorage.setItem("calden_default_duration",String(state.defaultDuration));
  localStorage.setItem("calden_scroll_now",String(state.scrollNow));
  $("#display-settings-status").textContent="Display settings saved.";
  renderCalendar();
});
$("#reset-display-settings").addEventListener("click",async()=>{
  try{
    await api("/api/me/week-start",{method:"PUT",body:JSON.stringify({week_start:"default"})});
    state.me.week_start=null;
  }catch(err){
    $("#display-settings-status").textContent=err.message;
    return;
  }
  state.defaultCalendar="";state.defaultDuration=60;state.scrollNow=true;
  $("#settings-personal-week-start").value="default";
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
  if(!e.target.files?.[0])return;
  previewAvatarFile(e.target,$("#settings-avatar-preview"),$("#settings-avatar-status"));
});
$("#save-settings-avatar").addEventListener("click",async()=>{
  const file=$("#settings-avatar-file").files?.[0];
  if(!file){$("#settings-avatar-status").textContent="Choose a profile image first.";return}
  if(file.size>maxAvatarUploadBytes){$("#settings-avatar-status").textContent="Profile image must be 10 MB or smaller.";return}
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

const googleRepairInput=$("#google-repair-export");
googleRepairInput?.addEventListener("change",()=>{state.googleRepairFile=googleRepairInput.files?.[0]||null;state.googleRepairPreview=null;$("#google-repair-confirm-wrap")?.classList.add("hidden");$("#repair-google-duplicates").disabled=true});
$("#scan-google-duplicates")?.addEventListener("click",scanGoogleDuplicateData);
$("#preview-google-repair")?.addEventListener("click",previewGoogleDuplicateRepair);
$("#repair-google-duplicates")?.addEventListener("click",repairGoogleDuplicateData);
$("#google-repair-confirmation")?.addEventListener("input",()=>{
  const body=state.googleRepairPreview;if(!body)return;
  const phrase=`REPAIR ${Number(body.export_repairable_groups)||0}`;
  $("#repair-google-duplicates").disabled=$("#google-repair-confirmation").value!==phrase;
});

const googleImportInput=$("#google-calendar-export");
const googleImportDrop=$("#google-import-drop");
googleImportInput?.addEventListener("change",()=>setGoogleImportFile(googleImportInput.files?.[0]||null));
googleImportDrop?.addEventListener("dragover",e=>{e.preventDefault();googleImportDrop.classList.add("drag-over")});
googleImportDrop?.addEventListener("dragleave",()=>googleImportDrop.classList.remove("drag-over"));
googleImportDrop?.addEventListener("drop",e=>{
  e.preventDefault();googleImportDrop.classList.remove("drag-over");
  const file=e.dataTransfer?.files?.[0];if(file)setGoogleImportFile(file);
});
$("#review-google-calendar")?.addEventListener("click",async()=>{
  const file=state.googleImportFile||googleImportInput?.files?.[0];
  const status=$("#google-import-status"),button=$("#review-google-calendar"),results=$("#google-import-results");
  if(!file){status.textContent="Choose your Google Calendar export first.";return}
  button.disabled=true;button.textContent="Reviewing…";status.textContent="Comparing Google calendars with the calendars already in Calden…";
  results.classList.add("hidden");results.innerHTML="";
  try{
    const form=new FormData();form.append("archive",file,file.name);
    const res=await fetch("/api/integrations/google/preview",{method:"POST",headers:{Authorization:"Bearer "+state.token},body:form});
    const body=await res.json().catch(()=>null);
    if(!res.ok)throw new Error(body?.error||"Could not review Google Calendar export");
    renderGoogleImportMapping(body);
    const suppressed=Number(body.duplicate_events_suppressed)||0;
    status.textContent=`Found ${body.calendar_count||0} Google calendar${Number(body.calendar_count)===1?"":"s"}${suppressed?" · "+suppressed+" stale duplicate event copy"+(suppressed===1?"":"ies")+" detected and suppressed":""}. Review each destination below before importing.`;
  }catch(err){
    status.textContent=err.message;
  }finally{
    button.disabled=false;button.textContent="Review calendars";
  }
});
function renderGoogleDuplicateSummary(body){
  const host=$("#google-duplicate-summary"),workflow=$("#google-repair-workflow"),examples=$("#google-duplicate-examples");
  const groups=Number(body?.duplicate_groups)||0,copies=Number(body?.extra_copies)||0;
  host.classList.remove("hidden");
  host.innerHTML=groups
    ?`<div><strong>${groups}</strong><span>duplicate UID group${groups===1?"":"s"}</span></div><div><strong>${copies}</strong><span>redundant Calden event cop${copies===1?"y":"ies"}</span></div><div><strong>${Number(body?.export_needed)||0}</strong><span>need export ownership check</span></div>`
    :'<div class="google-repair-clean"><strong>No duplicate imported UIDs found.</strong><span>Calden did not find multiple stored events sharing the same imported Google UID.</span></div>';
  workflow.classList.toggle("hidden",groups===0);
  examples.innerHTML=(body?.examples||[]).map(item=>`<div class="google-duplicate-example"><strong>${escapeHTML(item.title||"Untitled event")}</strong><span>${Number(item.copies)||0} copies · ${escapeHTML((item.calendars||[]).join(" / "))}</span></div>`).join("");
}

async function scanGoogleDuplicateData(){
  const button=$("#scan-google-duplicates"),status=$("#google-repair-status");
  button.disabled=true;button.textContent="Scanning…";
  try{
    const body=await api("/api/integrations/google/duplicates");
    state.googleRepairPreview=null;
    renderGoogleDuplicateSummary(body);
    status.textContent=Number(body.duplicate_groups)?"Duplicates found in Calden. Choose the original Google export to determine the correct surviving copy.":"No duplicate imported Google UIDs were found.";
    $("#google-repair-confirm-wrap").classList.add("hidden");
    $("#repair-google-duplicates").disabled=true;
  }catch(err){status.textContent=err.message}
  finally{button.disabled=false;button.textContent="Scan Calden"}
}

async function previewGoogleDuplicateRepair(){
  const file=state.googleRepairFile||$("#google-repair-export")?.files?.[0]||state.googleImportFile;
  const status=$("#google-repair-status"),button=$("#preview-google-repair");
  if(!file){status.textContent="Choose the original Google Calendar export first.";return}
  button.disabled=true;button.textContent="Checking…";status.textContent="Comparing the existing Calden duplicates with the original Google export…";
  try{
    const form=new FormData();form.append("archive",file,file.name);
    const res=await fetch("/api/integrations/google/duplicates/preview",{method:"POST",headers:{Authorization:"Bearer "+state.token},body:form});
    const body=await res.json().catch(()=>null);if(!res.ok)throw new Error(body?.error||"Could not preview duplicate repair");
    state.googleRepairPreview=body;renderGoogleDuplicateSummary(body);
    const repairable=Number(body.export_repairable_groups)||0;
    if(!repairable){status.textContent="Calden could not safely identify a canonical copy for the remaining duplicates. Nothing has been changed.";return}
    const phrase=`REPAIR ${repairable}`;
    $("#google-repair-phrase").textContent=phrase;
    $("#google-repair-confirmation").value="";
    $("#google-repair-confirm-wrap").classList.remove("hidden");
    $("#repair-google-duplicates").disabled=true;
    status.textContent=`${repairable} duplicate group${repairable===1?" is":"s are"} safely repairable in place. No calendars will be deleted and no events will be imported.`;
  }catch(err){status.textContent=err.message}
  finally{button.disabled=false;button.textContent="Preview repair"}
}

async function repairGoogleDuplicateData(){
  const body=state.googleRepairPreview,file=state.googleRepairFile||$("#google-repair-export")?.files?.[0]||state.googleImportFile;
  if(!body||!file)return;
  const phrase=`REPAIR ${Number(body.export_repairable_groups)||0}`;
  if($("#google-repair-confirmation").value!==phrase)return;
  const button=$("#repair-google-duplicates"),status=$("#google-repair-status");
  button.disabled=true;button.textContent="Repairing…";
  try{
    const form=new FormData();form.append("archive",file,file.name);
    const res=await fetch("/api/integrations/google/duplicates/repair",{method:"POST",headers:{Authorization:"Bearer "+state.token},body:form});
    const result=await res.json().catch(()=>null);if(!res.ok)throw new Error(result?.error||"Could not repair duplicate events");
    await reloadSharedData();renderCalendar();renderAgenda();renderBills();
    renderGoogleDuplicateSummary(result);
    $("#google-repair-confirm-wrap").classList.add("hidden");
    status.textContent=`Removed ${Number(result.removed_copies)||0} redundant Calden event cop${Number(result.removed_copies)===1?"y":"ies"}. Calendars and nonduplicate events were left in place.`;
  }catch(err){status.textContent=err.message}
  finally{button.textContent="Repair duplicates"}
}

$("#import-google-calendar")?.addEventListener("click",async()=>{
  const file=state.googleImportFile||googleImportInput?.files?.[0];
  const status=$("#google-import-status"),button=$("#import-google-calendar"),results=$("#google-import-results");
  if(!file){status.textContent="Choose your Google Calendar export first.";return}
  if(!state.googleImportPreview?.length){status.textContent="Review the calendars before importing.";return}
  button.disabled=true;button.textContent="Importing…";status.textContent="Importing the calendar mapping you approved…";
  results.classList.add("hidden");results.innerHTML="";
  try{
    const form=new FormData();
    form.append("archive",file,file.name);
    form.append("calendar_mapping",JSON.stringify(collectGoogleMapping()));
    const res=await fetch("/api/integrations/google/import",{method:"POST",headers:{Authorization:"Bearer "+state.token},body:form});
    const body=await res.json().catch(()=>null);
    if(!res.ok)throw new Error(body?.error||"Google Calendar import failed");
    const cleanup=Number(body.cleaned_calendars)||0;
    const reconciled=Number(body.duplicate_events_reconciled)||0;
    const suppressed=Number(body.duplicate_events_suppressed)||0;
    status.textContent=`Imported ${body.calendar_count||0} calendar${Number(body.calendar_count)===1?"":"s"}: ${body.created||0} new events, ${body.updated||0} updated${suppressed?" · "+suppressed+" stale Google event copy"+(suppressed===1?"":"ies")+" suppressed":""}${reconciled?" · "+reconciled+" existing duplicate event"+(reconciled===1?"":"s")+" repaired":""}${cleanup?" · "+cleanup+" old duplicate calendar"+(cleanup===1?"":"s")+" removed":""}.`;
    renderGoogleImportResults(body);
    await reloadSharedData();
    renderCalendars();renderCategories();renderBillNavigation();renderBills();renderEventControls();renderCalendar();renderAgenda();renderNotifications();renderSettings();
  }catch(err){
    status.textContent=err.message;
  }finally{
    button.disabled=false;button.textContent="Import selected calendars";
  }
});

$("#monita-load-channels")?.addEventListener("click",async()=>{
  const button=$("#monita-load-channels"),form=$("#monita-form"),status=$("#monita-status");
  button.disabled=true;status.textContent="Loading Monita channels…";
  try{
    const result=await api("/api/integrations/monita/channels",{
      method:"POST",body:JSON.stringify({server_url:form.server_url.value,token:form.token.value})
    });
    const select=form.channel_id,chosen=select.value!=="0"?select.value:select.dataset.selected||"0";
    select.innerHTML='<option value="0">Select a channel</option>';
    for(const channel of result.channels||[]){
      const option=document.createElement("option");
      option.value=String(channel.id);option.textContent=channel.name||("Channel "+channel.id);
      select.append(option);
    }
    if([...select.options].some(option=>option.value===chosen))select.value=chosen;
    status.textContent=(result.channels||[]).length+" available Monita channel(s) loaded.";
  }catch(err){status.textContent=err.message}
  finally{button.disabled=false}
});

async function loadPrivateBusyTime(){
  const list=$("#busy-block-list"),status=$("#busy-block-status");
  try{
    const result=await api("/api/scheduling/busy");
    const items=result.items||[];
    list.innerHTML=items.length?items.map(item=>{
      const start=new Date(item.starts_at).toLocaleString();
      const end=new Date(item.ends_at).toLocaleString();
      return `<div class="row"><span>${escapeHTML(start)} – ${escapeHTML(end)} · ${escapeHTML(item.label||"Busy")}</span><button type="button" class="button secondary compact" data-busy-remove="${escapeAttr(item.id)}">Remove</button></div>`;
    }).join(""):'<p class="muted">No private busy blocks.</p>';
    status.textContent="";
  }catch(error){status.textContent=error.message}
}
$("#busy-block-form").addEventListener("submit",async event=>{
  event.preventDefault();
  const form=event.currentTarget,fd=new FormData(form);
  const status=$("#busy-block-status");
  try{
    const start=new Date(fd.get("starts_at")),end=new Date(fd.get("ends_at"));
    if(!Number.isFinite(start.getTime())||!Number.isFinite(end.getTime())||
      end<=start)throw Error("End time must be later than start time.");
    await api("/api/scheduling/busy",{method:"POST",body:JSON.stringify({
      starts_at:start.toISOString(),ends_at:end.toISOString(),
      label:String(fd.get("label")||"")
    })});
    form.reset();await loadPrivateBusyTime();status.textContent="Private busy time saved.";
  }catch(error){status.textContent=error.message}
});
$("#busy-block-list").addEventListener("click",async event=>{
  const button=event.target.closest("[data-busy-remove]");if(!button)return;
  button.disabled=true;
  try{await api("/api/scheduling/busy/"+encodeURIComponent(button.dataset.busyRemove),
    {method:"DELETE"});await loadPrivateBusyTime()}
  catch(error){$("#busy-block-status").textContent=error.message;button.disabled=false}
});
$("#connect-monita").addEventListener("click",async()=>{
  const form=$("#monita-form"),status=$("#monita-status"),button=$("#connect-monita");
  if(!form.channel_id.value || form.channel_id.value==="0"){
    status.textContent="Load and select an authorized Monita channel first.";return;
  }
  button.disabled=true;status.textContent="Connecting and verifying channel access…";
  try{
    const payload={server_url:form.server_url.value,token:form.token.value,
      channel_id:Number(form.channel_id.value),enabled:true};
    await api("/api/integrations/monita/connect",{method:"POST",body:JSON.stringify(payload)});
    status.textContent="Monita connected. The selected channel received a test notification.";
    await loadMonita();
  }catch(err){status.textContent=err.message}
  finally{button.disabled=false}
});
$("#disconnect-monita").addEventListener("click",async()=>{
  const status=$("#monita-status");
  if(!confirm("Disconnect Monita? This clears the saved service token and disables Monita reminders."))return;
  status.textContent="Disconnecting Monita…";
  try{
    await api("/api/integrations/monita/disconnect",{method:"POST",body:"{}"});
    await loadMonita();
    status.textContent="Monita disconnected. Credential cleared.";
  }catch(err){status.textContent=err.message}
});
$("#monita-form").addEventListener("submit",async e=>{
  e.preventDefault();const form=e.currentTarget;$("#monita-status").textContent="";
  const payload={enabled:form.enabled.checked,server_url:form.server_url.value,token:form.token.value,default_channel:form.default_channel.value,channel_id:Number(form.channel_id.value||0)};
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
  const input=$("#restore-file"),file=input.files?.[0];if(!file){$("#restore-status").textContent="Choose a Calden backup file first.";return}
  if(!confirm("Validate and stage this backup for restore? Nothing will be changed until Calden restarts."))return;
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
  if(!confirm("Restart Calden now and apply the staged restore? A pre-restore safety backup will be created automatically."))return;
  $("#restore-status").textContent="Restarting Calden to apply restore…";
  try{
    await api("/api/system/restart",{method:"POST",body:"{}"});
    let attempts=0;
    const wait=async()=>{
      attempts++;
      try{
        const res=await fetch("/api/health",{cache:"no-store"});
        if(res.ok){location.reload();return}
      }catch{}
      if(attempts<90)setTimeout(wait,1500);else $("#restore-status").textContent="Calden has not returned yet. Check container logs.";
    };
    setTimeout(wait,1800);
  }catch(err){$("#restore-status").textContent=err.message}
});

$("#activity-filter").addEventListener("change",loadActivity);
$("#refresh-activity").addEventListener("click",loadActivity);
$("#check-updates").addEventListener("click",loadUpdater);
["update-channel","update-auto-check"].forEach(id=>$("#"+id).addEventListener("change",()=>{state.updatePrefsDirty=true}));
$("#save-update-prefs").addEventListener("click",async()=>{
  $("#update-pref-status").textContent="";
  try{
    await api("/api/system/update/preferences",{method:"PUT",body:JSON.stringify({channel:$("#update-channel").value,auto_check:$("#update-auto-check").checked})});
    state.updatePrefsDirty=false;
    $("#update-pref-status").textContent="Update preferences saved.";await loadUpdater();
  }catch(err){$("#update-pref-status").textContent=err.message}
});
$("#apply-update").addEventListener("click",async()=>{
  const version=$("#apply-update").dataset.version;if(!version)return;
  if(!confirm(`Install Calden ${version}? Calden will create a database safety backup and restart itself.`))return;
  try{const status=await api("/api/system/update/install",{method:"POST",body:JSON.stringify({version})});state.updateInfo.status=status;renderUpdater(state.updateInfo);startUpdatePolling()}
  catch(err){$("#update-state").textContent=err.message}
});
$("#rollback-update").addEventListener("click",async()=>{
  if(!confirm("Roll back to the previous Calden runtime? Calden will restart."))return;
  try{const status=await api("/api/system/update/rollback",{method:"POST",body:"{}"});state.updateInfo.status=status;renderUpdater(state.updateInfo);startUpdatePolling()}
  catch(err){$("#update-state").textContent=err.message}
});


function renderChores(){
  const tasksEnabled=!!state.settings?.tasks_enabled;
  const choresEnabled=!!state.settings?.chores_enabled;
  const enabled=tasksEnabled||choresEnabled;
  document.querySelectorAll(".chore-nav").forEach(el=>{
    el.classList.toggle("hidden",!enabled);
    // Preserve the SVG added by decorateNavigation. textContent on the link
    // would remove the icon whenever feature visibility changes.
    const label=el.querySelector(".nav-label");
    const defaultName=tasksEnabled&&choresEnabled?"Tasks & Chores":tasksEnabled?"Tasks":"Chores";
    const configured=window.caldenAppearance?.label("chores","Chores")||"Chores";
    if(label)label.textContent=configured==="Chores"?defaultName:configured;
  });
  document.querySelectorAll(".chore-manager").forEach(el=>el.classList.toggle("hidden",!["admin","planner"].includes(state.me?.role)));
  $("#chore-page-title").textContent=tasksEnabled&&choresEnabled?"Tasks & Chores":tasksEnabled?"Tasks":"Chores";
  $("#chore-add").textContent=tasksEnabled&&choresEnabled?"+ Add task or chore":tasksEnabled?"+ Add task":"+ Add chore";
  const kindField=$("#chore-kind"),selected=kindField.value;
  kindField.querySelectorAll("option").forEach(option=>{option.disabled=option.value==="task"?!tasksEnabled:!choresEnabled});
  if(kindField.selectedOptions[0]?.disabled){kindField.value=tasksEnabled?"task":"chore"}
  if(!enabled){$("#chore-list").innerHTML="";return;}
  const all=(state.chores||[]).filter(t=>t.kind==="task"?tasksEnabled:choresEnabled),filter=$("#chore-filter").value,mine=state.me?.id;
  const manage=["admin","planner"].includes(state.me?.role);
  const shown=all.filter(t=>filter==="mine"?(t.assignee_ids||[t.assigned_to]).includes(mine):filter==="open"?t.status!=="done":filter==="done"?t.status==="done":true)
    .sort((a,b)=>a.due_date.localeCompare(b.due_date)||a.title.localeCompare(b.title));
  const done=all.filter(t=>t.status==="done").length;
  $("#chore-summary").innerHTML=`<div class="chore-summary-item"><strong>${all.length-done}</strong><span>To do</span></div><div class="chore-summary-item"><strong>${done}</strong><span>Finished</span></div><div class="chore-summary-item"><strong>${all.length}</strong><span>Occurrences</span></div>`;
  const list=$("#chore-list");
  if(!shown.length){list.innerHTML='<div class="panel chore-empty">No chores in this view.</div>';return}
  const d=new Date(),today=[d.getFullYear(),String(d.getMonth()+1).padStart(2,"0"),String(d.getDate()).padStart(2,"0")].join("-");
  const labels={none:"One time",daily:"Every day",weekly:"Weekly",biweekly:"Every 2 weeks",monthly:"Monthly",yearly:"Yearly"};
  list.innerHTML=shown.map(t=>{
    const overdue=t.status!=="done"&&t.due_date<today;
    const date=new Date(t.due_date+"T12:00:00").toLocaleDateString(undefined,{month:"short",day:"numeric",year:"numeric"});
    const participants=Array.isArray(t.assignees)&&t.assignees.length?t.assignees:[{id:t.assigned_to,name:t.assignee_name,status:t.status,avatar_url:t.assignee_avatar_url,initials:t.assignee_initials}];
    const personRows=participants.map(p=>{
      const editable=manage||p.id===mine;
      const owner=state.users.find(u=>u.id===p.id)||{display_name:p.name,avatar_url:p.avatar_url,initials:p.initials};
      const progress=editable?`<select aria-label="Progress for ${escapeAttr(owner.display_name||p.name)}" data-chore-status="${escapeAttr(t.id)}" data-chore-person="${escapeAttr(p.id)}" data-chore-date="${escapeAttr(t.occurrence_date||t.due_date)}"><option value="pending" ${p.status==="pending"?"selected":""}>Pending</option><option value="in_progress" ${p.status==="in_progress"?"selected":""}>In progress</option><option value="done" ${p.status==="done"?"selected":""}>Done</option></select>`:`<span>${p.status==="done"?"Done":p.status==="in_progress"?"In progress":"Pending"}</span>`;
      return `<div class="chore-assignee-progress"><i class="avatar">${avatarMarkup(owner)}</i><strong>${escapeHTML(owner.display_name||p.name||"Household member")}</strong>${progress}</div>`;
    }).join("");
    const management=manage?`<div class="chore-actions"><button class="button secondary compact" data-chore-edit="${escapeAttr(t.id)}" type="button">Edit series</button><button class="button secondary compact" data-chore-delete="${escapeAttr(t.id)}" type="button">Delete series</button></div>`:"";
    return `<article class="panel chore-card ${overdue?"chore-overdue":""}"><div><h3>${escapeHTML(t.title)} <span class="chore-repeat-tag">${t.kind==="task"?"Task":"Chore"}</span></h3><p class="muted">${escapeHTML(t.notes||"")}</p><div class="chore-meta"><span>Due ${escapeHTML(date)}</span><span class="chore-repeat-tag">${labels[t.recurrence_rule]||"One time"}</span>${overdue?'<span class="chore-state">Overdue</span>':""}${state.settings?.rewards_enabled===true&&t.kind==="chore"&&t.reward_points>0?`<span class="chore-repeat-tag">${t.reward_points} points on approval</span>`:""}</div><div class="chore-assignee-list">${personRows}</div>${management}</div></article>`;
  }).join("");
}
async function loadChores(){
  if(!state.settings?.chores_enabled&&!state.settings?.tasks_enabled)return;
  try{const result=await api("/api/chores");state.chores=result.items||[];$("#chore-status").textContent="";renderChores()}
  catch(err){$("#chore-status").textContent=err.message}
}
function updateChoreRepeat(){
  const form=$("#chore-form");
  const repeat=form.elements.namedItem("recurrence_rule").value!=="none";
  $("#chore-repeat-until-wrap").classList.toggle("hidden",!repeat);
  if(!repeat)form.elements.namedItem("recurrence_until").value="";
}
function openChoreEditor(chore=null){
  if(!["admin","planner"].includes(state.me?.role))return;
  const form=$("#chore-form"),field=name=>form.elements.namedItem(name);
  form.reset();
  field("id").value=chore?.id||"";
  field("kind").value=chore?.kind||(state.settings?.tasks_enabled?"task":"chore");
  if(field("kind").selectedOptions[0]?.disabled)field("kind").value=state.settings?.tasks_enabled?"task":"chore";
  field("title").value=chore?.title||"";
  field("notes").value=chore?.notes||"";
  field("reward_points").value=String(chore?.reward_points||0);
  const today=new Date(),day=[today.getFullYear(),String(today.getMonth()+1).padStart(2,"0"),String(today.getDate()).padStart(2,"0")].join("-");
  field("due_date").value=chore?.series_due_date||chore?.due_date||day;
  field("recurrence_rule").value=chore?.recurrence_rule||"none";
  field("recurrence_until").value=chore?.recurrence_until||"";
  updateChoreRepeat();
  $("#chore-form").querySelector(".reward-chore-field").classList.toggle("hidden",field("kind").value!=="chore"||state.settings?.rewards_enabled!==true);
  const assigned=new Set(chore?.assignee_ids||[chore?.assigned_to||state.me?.id]);
  $("#chore-assignees").innerHTML=state.users.filter(u=>u.active!==false).map(u=>`<label class="chore-member-choice"><input type="checkbox" name="assignee_id" value="${escapeAttr(u.id)}" ${assigned.has(u.id)?"checked":""}><i class="avatar">${avatarMarkup(u)}</i><span>${escapeHTML(u.display_name)}</span></label>`).join("");
  $("#chore-editor-heading").textContent=chore?"Edit "+(chore.kind==="task"?"task":"chore"):state.settings?.tasks_enabled&&state.settings?.chores_enabled?"Add task or chore":state.settings?.tasks_enabled?"Add task":"Add chore";
  $("#chore-form button[type=submit]").textContent="Save "+(field("kind").value==="task"?"task":"chore");
  $("#chore-form-status").textContent="";
  $("#chore-editor-panel").classList.remove("hidden");
  field("title").focus();
}
$("#chore-add").addEventListener("click",()=>openChoreEditor());
$("#chore-cancel").addEventListener("click",()=>$("#chore-editor-panel").classList.add("hidden"));
$("#chore-refresh").addEventListener("click",loadChores);
$("#chore-filter").addEventListener("change",renderChores);
$("#chore-form").elements.namedItem("recurrence_rule").addEventListener("change",updateChoreRepeat);
$("#chore-kind").addEventListener("change",()=>{
  $("#chore-form").querySelector(".reward-chore-field").classList.toggle("hidden",$("#chore-kind").value!=="chore"||state.settings?.rewards_enabled!==true);
  $("#chore-form button[type=submit]").textContent="Save "+($("#chore-kind").value==="task"?"task":"chore");
});
$("#chore-form").addEventListener("submit",async event=>{
  event.preventDefault();
  const form=event.currentTarget,field=name=>form.elements.namedItem(name),id=field("id").value;
  const assignee_ids=[...form.querySelectorAll('input[name="assignee_id"]:checked')].map(input=>input.value);
  if(!assignee_ids.length){$("#chore-form-status").textContent="Choose at least one person.";return}
  const payload={kind:field("kind").value,title:field("title").value.trim(),notes:field("notes").value,due_date:field("due_date").value,assignee_ids,...(state.settings?.rewards_enabled===true?{reward_points:field("kind").value==="chore"?Number(field("reward_points").value):0}:{}),recurrence_rule:field("recurrence_rule").value,recurrence_until:field("recurrence_until").value};
  try{
    await api(id?"/api/chores/"+encodeURIComponent(id):"/api/chores",{method:id?"PUT":"POST",body:JSON.stringify(payload)});
    $("#chore-editor-panel").classList.add("hidden");await loadChores();
  }catch(err){$("#chore-form-status").textContent=err.message}
});
$("#chore-list").addEventListener("click",async event=>{
  const edit=event.target.closest("[data-chore-edit]"),del=event.target.closest("[data-chore-delete]");
  if(edit){openChoreEditor(state.chores.find(t=>t.id===edit.dataset.choreEdit));return}
  if(del){
    if(!confirm("Delete this entire chore series and its completion history?"))return;
    try{await api("/api/chores/"+encodeURIComponent(del.dataset.choreDelete),{method:"DELETE"});await loadChores()}
    catch(err){$("#chore-status").textContent=err.message}
  }
});
$("#chore-list").addEventListener("change",async event=>{
  const select=event.target.closest("[data-chore-status]");if(!select)return;
  select.disabled=true;
  try{
    await api("/api/chores/"+encodeURIComponent(select.dataset.choreStatus)+"/status",{method:"PUT",body:JSON.stringify({status:select.value,assignee_id:select.dataset.chorePerson,occurrence_date:select.dataset.choreDate})});
    await loadChores();
  }catch(err){$("#chore-status").textContent=err.message;await loadChores()}
});
// Each checkbox saves only its own global setting. The API returns all four,
// keeping the other three unchanged. This works for every household member.
for(const [name,label] of [["bills","Bills"],["agenda","Agenda"],["tasks","Tasks"],["chores","Chores"],["shopping","Shopping"],["meals","Meal Planning"],["packages","Packages"],["routines","Routines"],["rewards","Rewards & Allowances"],["school","School"],["maintenance","Home upkeep"],["pets","Pets"]]){
  $("#settings-"+name+"-enabled").addEventListener("change",async event=>{
    if(state.me?.role!=="admin")return;
    const control=event.currentTarget,status=$("#settings-feature-status"),value=control.checked;
    control.disabled=true;status.textContent="Saving "+label+"…";
    try{
      state.settings=await api("/api/settings/features",{method:"PUT",body:JSON.stringify({[name+"_enabled"]:value})});
      renderBillNavigation();
      renderChores();
      if(name==="packages")window.caldenPackages?.onToggle();
      $(".shopping-nav").forEach(el=>el.classList.toggle("hidden",state.settings?.shopping_enabled===false));
  $$(".meals-nav").forEach(el=>el.classList.toggle("hidden",state.settings?.meals_enabled===false||state.me?.is_child||state.me?.role==="restricted"));
      $$(".routines-nav").forEach(el=>el.classList.toggle("hidden",state.settings?.routines_enabled!==true));
      if(name==="shopping"&&state.settings.shopping_enabled===false&&state.currentPage==="shopping")navigate("calendar");
      if(name==="meals"&&state.settings.meals_enabled===false&&state.currentPage==="meals")navigate("dashboard");
      if(name==="meals")loadRecipeCoveIntegrationStatus();
      if(name==="routines"&&state.settings.routines_enabled!==true&&state.currentPage==="routines")navigate("calendar");
      $$(".rewards-nav").forEach(el=>el.classList.toggle("hidden",state.settings?.rewards_enabled!==true));
      window.caldenCare?.renderNav();
      if(["school","maintenance","pets"].includes(name)&&!state.settings[name+"_enabled"]&&state.currentPage===name)navigate("calendar");
      if(name==="rewards"&&state.settings.rewards_enabled!==true&&state.currentPage==="rewards")navigate("calendar");
      window.caldenFamily?.renderDashboard();
      if(!state.settings.chores_enabled&&!state.settings.tasks_enabled&&state.currentPage==="chores"){
        $("#chore-editor-panel").classList.add("hidden");navigate("calendar");
      }
      if(name==="tasks"||name==="chores"){
        $("#chore-editor-panel").classList.add("hidden");
        if(state.settings.tasks_enabled||state.settings.chores_enabled)await loadChores();else state.chores=[];
        renderChores();
      }
      status.textContent=label+(value?" enabled":" disabled")+" for everyone. Your saved information is unchanged.";
    }catch(err){
      control.checked=!value;
      status.textContent="Couldn't save "+label+": "+err.message;
    }finally{control.disabled=false}
  });
}

resetPersonForm();
resetCalendarForm();
resetCategoryForm();
boot().catch(err=>{console.error("Calden failed before authentication",err);showBootFailure(err);caldenBooting=false});


;(function familyUI() {
  // This controller is appended to the existing Calden web client.
  // All data-changing operations use the existing authenticated api() helper.
  const labels={calendar:"Upcoming events",notifications:"My reminders",chores:"Chores",tasks:"Tasks",bills:"Bills",agenda:"Family agenda",shopping:"Shopping lists",routines:"My routines",rewards:"My rewards"};
  const titleOf=kind=>window.caldenAppearance?.label(kind,labels[kind])||labels[kind];
  const status=(id,value)=>{const el=$(id);if(el)el.textContent=value||""};
  const esc=value=>escapeHTML(value??"");
  const canShop=()=>state.me?.role!=="restricted";
  const selectedList=()=>state.shopping.lists.find(item=>item.id===state.shopping.listID);
  const availableWidget=kind=>{
    if(state.me?.is_child){
      const allowed=new Set(state.me.child_modules||[]);
      const module=kind==="agenda"?"calendar":kind;
      if(!allowed.has(module))return false;
    }
    if(kind==="chores")return !!state.settings?.chores_enabled;
    if(kind==="shopping")return state.settings?.shopping_enabled!==false;
    if(kind==="routines")return state.settings?.routines_enabled===true;
    if(kind==="rewards")return state.settings?.rewards_enabled===true;
    if(kind==="tasks")return !!state.settings?.tasks_enabled;
    if(kind==="bills")return state.settings?.bills_enabled!==false;
    if(kind==="agenda")return state.settings?.agenda_enabled!==false;
    return Object.hasOwn(labels,kind);
  };
  const preference=()=>state.dashboard||{appearance:"system",layout:"standard",widgets:["calendar","notifications"]};
  function applyDashboardAppearance(){
    const pref=preference();
    document.body.classList.toggle("calden-family-dark",pref.appearance==="dark"||(pref.appearance==="system"&&window.matchMedia?.("(prefers-color-scheme: dark)").matches));
    document.body.classList.toggle("calden-family-simple",pref.layout==="simple");
  }
  const widgetList=(items,empty)=>{
    if(!items.length)return `<p class="family-widget-empty">${esc(empty)}</p>`;
    return `<ul class="family-widget-list">${items.map(item=>`<li><strong>${esc(item.title)}</strong><span>${esc(item.detail)}</span></li>`).join("")}</ul>`;
  };
  function dashboardContent(kind){
    if(kind==="calendar"||kind==="agenda"||kind==="bills"){
      const now=Date.now();
      const events=visibleEvents().filter(event=>{
        const start=new Date(event.starts_at).getTime();
        return Number.isFinite(start)&&start>=now&&start<now+30*86400000&&(kind!=="bills"||isBillCalendar(event.calendar_id));
      }).sort((a,b)=>new Date(a.starts_at)-new Date(b.starts_at)).slice(0,6);
      return widgetList(events.map(e=>({title:e.title,detail:new Date(e.starts_at).toLocaleString(undefined,{dateStyle:"medium",timeStyle:"short"})+" · "+(e.calendar_name||"Family")})),"Nothing upcoming in the next 30 days.");
    }
    if(kind==="notifications"){
      const items=(state.notifications||[]).filter(x=>!x.read_at).slice(0,5);
      return widgetList(items.map(x=>({title:x.title||"Reminder",detail:x.message||"New family notification"})),"You're all caught up.");
    }
    if(kind==="tasks"||kind==="chores"){
      const own=(state.chores||[]).filter(c=>(c.kind||"chore")===(kind==="tasks"?"task":"chore")
        &&(c.assignee_ids||[c.assigned_to]).includes(state.me?.id)).slice(0,6);
      return widgetList(own.map(c=>({title:c.title,detail:"Due "+(c.occurrence_date||c.due_date||"soon")})),"No assigned items right now.");
    }
    if(kind==="rewards"){
      const mine=(state.rewards?.balances||[]).find(x=>x.user_id===state.me?.id);
      return widgetList([{title:String(mine?.points||0)+" points available",detail:"Rewards and allowance activity"},{title:"Allowance balance: "+money((mine?.allowance_balance_cents||0)/100),detail:"Recorded as due or paid"}],"No reward activity yet.");
    }
    if(kind==="routines"){
      const rows=(state.routines||[]).filter(r=>r.due_today).slice(0,6);
      return widgetList(rows.map(r=>({title:r.title,detail:r.done+" of "+r.total+" steps · "+r.assignee_name})),"No routines for today.");
    }
    if(kind==="shopping"){
      const lists=(state.shopping.lists||[]).filter(x=>!x.archived).slice(0,6);
      return widgetList(lists.map(l=>({title:l.name,detail:l.remaining+" still needed · "+(l.shared?"Shared":"Private")})),"No shopping lists yet.");
    }
    return "";
  }
  function renderDashboard(){
    if(!state.me)return;
    applyDashboardAppearance();
    const host=$("#dashboard-widgets");if(!host)return;
    const pref=preference();
    $("#dashboard-appearance").value=pref.appearance;
    // The global Settings selector and dashboard editor share a single saved
    // account preference. Never treat either control as its own independent theme.
    if($("#settings-appearance"))$("#settings-appearance").value=pref.appearance;
    $("#dashboard-layout").value=state.me?.is_child?(state.me.child_layout||"simple"):pref.layout;
    $("#dashboard-layout").disabled=!!state.me?.is_child;
    $("#dashboard-widget-choices").innerHTML=Object.entries(labels).filter(([kind])=>availableWidget(kind))
      .map(([kind,label])=>`<label><input type="checkbox" data-dashboard-choice="${kind}" ${pref.widgets.includes(kind)?"checked":""}>${esc(label)}</label>`).join("");
    const widgets=pref.widgets.filter(availableWidget);
    host.innerHTML=widgets.length?widgets.map((kind,index)=>{
      const page=kind==="rewards"?"rewards":kind==="routines"?"routines":kind==="shopping"?"shopping":kind==="notifications"?"notifications":kind==="tasks"||kind==="chores"?"chores":kind==="bills"?"bills":kind==="agenda"?"agenda":"calendar";
      return `<article class="family-widget" data-dashboard-widget="${kind}"><div class="family-widget-title"><h3>${esc(titleOf(kind))}</h3>
          <div class="family-widget-tools"><button type="button" class="button secondary compact" data-dashboard-move="${kind}" data-direction="-1" ${index===0?"disabled":""} aria-label="Move ${esc(titleOf(kind))} up">↑</button>
          <button type="button" class="button secondary compact" data-dashboard-move="${kind}" data-direction="1" ${index===widgets.length-1?"disabled":""} aria-label="Move ${esc(titleOf(kind))} down">↓</button></div></div>
          ${dashboardContent(kind)}
          <button type="button" class="button secondary compact" data-open-family-page="${page}" style="margin-top:14px">Open ${esc(titleOf(kind))}</button></article>`;
    }).join(""):`<div class="panel"><h3>Make this space yours</h3><p class="muted">Choose a widget to start building your home.</p></div>`;
  }
  async function saveDashboard(prefs){
    status("#dashboard-status","Saving…");
    try{
      const saved=await api("/api/me/dashboard",{method:"PUT",body:JSON.stringify(prefs)});
      state.dashboard=saved;renderDashboard();
      status("#dashboard-status","Saved for your account and your other devices.");
      return true;
    }catch(err){status("#dashboard-status","Couldn't save: "+err.message);return false}
  }
  function closeDashboardEditor(){
    $("#dashboard-config").classList.add("hidden");
    $("#dashboard-customize").setAttribute("aria-expanded","false");
  }
  $("#dashboard-customize").addEventListener("click",()=>{
    const open=$("#dashboard-config").classList.toggle("hidden")===false;
    $("#dashboard-customize").setAttribute("aria-expanded",String(open));
    renderDashboard();
  });
  $("#dashboard-cancel").addEventListener("click",closeDashboardEditor);
  // Theme is an app-wide personal preference; changing it must not reset
  // the dashboard widget order or the guardian-controlled child layout.
  $("#settings-appearance").addEventListener("change",async event=>{
    const selector=event.currentTarget;
    const original=preference();
    const next=selector.value;
    if(next===original.appearance)return;
    selector.disabled=true;
    status("#settings-appearance-status","Saving your appearance…");
    const saved=await saveDashboard({...original,appearance:next});
    if(saved){
      status("#settings-appearance-status",
        "Appearance saved. "+(next==="system"?"Calden now follows your device.":"Calden is now in "+(next==="light"?"Light":"Dark")+" mode."));
    }else{
      selector.value=original.appearance;
      status("#settings-appearance-status","Unable to save appearance. Your previous setting is unchanged.");
    }
    selector.disabled=false;
  });
  $("#dashboard-save").addEventListener("click",async()=>{
    const prefs=preference();
    const selected=new Set([...$$("[data-dashboard-choice]:checked")].map(x=>x.dataset.dashboardChoice));
    const widgets=[...prefs.widgets.filter(x=>selected.has(x)),...Object.keys(labels).filter(x=>selected.has(x)&&!prefs.widgets.includes(x))];
    const ok=await saveDashboard({appearance:$("#dashboard-appearance").value,layout:$("#dashboard-layout").value,widgets});
    if(ok)closeDashboardEditor();
  });
  $("#dashboard-widgets").addEventListener("click",async event=>{
    const move=event.target.closest("[data-dashboard-move]");
    if(move){
      const prefs=preference();
      const widgets=prefs.widgets.slice(),index=widgets.indexOf(move.dataset.dashboardMove),next=index+Number(move.dataset.direction);
      if(index<0||next<0||next>=widgets.length)return;
      [widgets[index],widgets[next]]=[widgets[next],widgets[index]];
      await saveDashboard({...prefs,widgets});
      return;
    }
    const open=event.target.closest("[data-open-family-page]");
    if(open)navigate(open.dataset.openFamilyPage);
  });
  const media=window.matchMedia?.("(prefers-color-scheme: dark)");
  media?.addEventListener?.("change",applyDashboardAppearance);
  // Update list summaries independently of item detail loading.
  async function loadShoppingLists(){
    const result=await api("/api/shopping/lists");
    state.shopping.lists=result.items||[];
    if(!state.shopping.lists.some(x=>x.id===state.shopping.listID))
      state.shopping.listID=state.shopping.lists.find(x=>!x.archived)?.id||state.shopping.lists[0]?.id||null;
    renderShopping();renderDashboard();
  }
  async function loadShoppingItems(){
    const id=state.shopping.listID;
    if(!id){state.shopping.items=[];renderShopping();return}
    const result=await api("/api/shopping/lists/"+encodeURIComponent(id)+"/items");
    if(state.shopping.listID!==id)return;
    state.shopping.items=result.items||[];
    renderShopping();
  }
  async function loadShopping(){
    try{
      status("#shopping-status","");
      await loadShoppingLists();
      await loadShoppingItems();
      await loadShoppingHistory();
    }catch(err){status("#shopping-status","Couldn't load shopping lists: "+err.message)}
  }
  function itemPayload(item,purchased=item.purchased){
    // Legacy-shaped updates preserve supply metadata on the server.
    return {name:item.name,quantity:Number(item.quantity),unit:item.unit,notes:item.notes||"",purchased:!!purchased};
  }
  function shoppingAssigneeName(id){
    return state.users.find(user=>user.id===id)?.display_name||"Unassigned member";
  }
  function populateShoppingAssignees(list){
    const select=$("#shopping-item-form").elements.namedItem("assigned_to");
    const current=select.value;
    const candidates=state.users.filter(u=>u.active!==false&&(list?.shared||u.id===list?.created_by));
    select.innerHTML='<option value="">Anyone in the household</option>'+
      candidates.map(u=>'<option value="'+esc(u.id)+'">'+esc(u.display_name)+'</option>').join("");
    if(candidates.some(u=>u.id===current))select.value=current;
  }
  function renderShoppingHistory(rows){
    const host=$("#shopping-history-list");
    if(!rows?.length){
      host.innerHTML='<p class="family-widget-empty">No changes recorded yet.</p>';return;
    }
    const names={added:"Added",updated:"Updated",purchased:"Purchased",needed:"Marked needed",removed:"Removed"};
    host.innerHTML=rows.map(entry=>{
      const when=new Date(entry.created_at);
      const date=Number.isNaN(when.getTime())?"Unknown date":when.toLocaleString();
      const actor=entry.performed_by?shoppingAssigneeName(entry.performed_by):"";
      const item=entry.item||{};
      return '<div class="family-history-entry"><strong>'+esc(names[entry.action]||"Changed")+
        ' · '+esc(item.name||"Shopping item")+'</strong><small>'+esc(date)+
        (actor?' · '+esc(actor):'')+'</small><span class="muted">'+
        esc([item.category,item.quantity&&item.unit?Number(item.quantity)+" "+item.unit:""].filter(Boolean).join(" · "))+
        '</span></div>';
    }).join("");
  }
  async function loadShoppingHistory(){
    const list=selectedList(),host=$("#shopping-history-list");
    if(!list||host.classList.contains("hidden"))return;
    const id=list.id;
    host.textContent="Loading list history…";
    try{
      const data=await api("/api/shopping/lists/"+encodeURIComponent(id)+"/history");
      if(selectedList()?.id!==id||host.classList.contains("hidden"))return;
      renderShoppingHistory(data.items||[]);
    }catch(err){
      if(selectedList()?.id===id)host.textContent="Could not load history: "+err.message;
    }
  }
  function renderShopping(){
    const picker=$("#shopping-list-picker"),host=$("#shopping-item-list");
    if(!picker||!host)return;
    $("#shopping-new-list").classList.toggle("hidden",!canShop());
    picker.innerHTML=state.shopping.lists.length?state.shopping.lists.map(l=>
      `<button type="button" class="family-list-choice ${l.id===state.shopping.listID?"active":""}"
        data-shopping-select="${esc(l.id)}" aria-current="${l.id===state.shopping.listID?"true":"false"}">
        <strong>${esc(l.name)}</strong><small>${l.archived?"Archived · ":""}${esc(String(l.remaining))} needed${Number(l.low_stock)>0?" · "+esc(String(l.low_stock))+" low stock":""} · ${l.shared?"Shared":"Private"}</small></button>`).join("")
      :`<p class="family-widget-empty">No lists yet. Add one to get started.</p>`;
    const list=selectedList();
    populateShoppingAssignees(list);
    $("#shopping-history-toggle").disabled=!list;
    $("#shopping-detail-name").textContent=list?.name||"Choose or create a list";
    $("#shopping-detail-caption").textContent=list?(list.shared?"Visible to everyone in the household":"Only visible to you"):"Shopping lists keep your family's purchases organized.";
    $("#shopping-edit-list").classList.toggle("hidden",!list?.can_manage);
    $("#shopping-delete-list").classList.toggle("hidden",!list?.can_manage);
    $("#shopping-item-form").classList.toggle("hidden",!list||!canShop()||list.archived);
    if(!list){host.innerHTML='<p class="family-widget-empty">Create a list to add items.</p>';return}
    const categoryControl=$("#shopping-category-filter");
    const currentCategory=categoryControl.value;
    const categories=[...new Set(state.shopping.items.map(x=>x.category||"").filter(Boolean))].sort((a,b)=>a.localeCompare(b));
    categoryControl.innerHTML='<option value="">All categories</option>'+
      categories.map(name=>'<option value="'+esc(name)+'">'+esc(name)+'</option>').join("");
    categoryControl.value=categories.includes(currentCategory)?currentCategory:"";
    const visibleItems=state.shopping.items.filter(x=>!categoryControl.value||x.category===categoryControl.value);
    const groups=[{label:"Still needed",items:visibleItems.filter(x=>!x.purchased)},{label:"Purchased",items:visibleItems.filter(x=>x.purchased)}];
    host.innerHTML=groups.map(group=>`<div class="family-section-title">${group.label} (${group.items.length})</div>`
      +(group.items.length?group.items.map(item=>`<div class="family-shopping-row ${item.purchased?"is-purchased":""}">
         <input type="checkbox" data-shopping-toggle="${esc(item.id)}" ${item.purchased?"checked":""} ${!canShop()||list.archived?"disabled":""} aria-label="Mark ${esc(item.name)} ${item.purchased?"needed":"purchased"}">
         <div><span class="family-shopping-name">${esc(item.name)}</span><span class="family-shopping-meta">${esc(Number(item.quantity).toString())} ${esc(item.unit)}${item.category?" · "+esc(item.category):""}${item.assigned_to?" · "+esc(shoppingAssigneeName(item.assigned_to)):""}${item.notes?" · "+esc(item.notes):""}</span>
         ${item.stock_on_hand!==null&&item.stock_on_hand!==undefined?'<span class="family-shopping-meta">In stock: '+esc(Number(item.stock_on_hand).toString())+` ${esc(item.unit)}`+(item.reorder_at!==null&&item.reorder_at!==undefined?' · Reorder at '+esc(Number(item.reorder_at).toString()):'')+'</span>':""}
         ${item.low_stock?'<span class="family-low-stock" aria-label="Supply running low">Running low</span>':""}</div>
         <div class="family-shopping-actions">${canShop()&&!list.archived?`<button class="button secondary compact" data-shopping-edit="${esc(item.id)}" type="button">Edit</button><button class="button secondary compact" data-shopping-delete="${esc(item.id)}" type="button">Remove</button>`:""}</div>
        </div>`).join(""):`<p class="family-widget-empty">No items here.</p>`)).join("");
  }
  function openShoppingListForm(list=null){
    if(!canShop())return;
    const form=$("#shopping-list-form");
    form.reset();
    form.elements.namedItem("id").value=list?.id||"";
    form.elements.namedItem("name").value=list?.name||"";
    form.elements.namedItem("shared").checked=list?list.shared:true;
    form.elements.namedItem("archived").checked=!!list?.archived;
    $("#shopping-list-form-title").textContent=list?"List settings":"New shopping list";
    form.classList.remove("hidden");
    form.elements.namedItem("name").focus();
  }
  function resetShoppingItemForm(){
    const form=$("#shopping-item-form");form.reset();
    form.elements.namedItem("id").value="";
    $("#shopping-item-submit").textContent="Add item";
    $("#shopping-item-cancel").classList.add("hidden");
  }
  $("#shopping-category-filter").addEventListener("change",renderShopping);
  $("#shopping-history-toggle").addEventListener("click",async()=>{
    const host=$("#shopping-history-list"),button=$("#shopping-history-toggle");
    const visible=host.classList.toggle("hidden")===false;
    button.setAttribute("aria-expanded",String(visible));
    button.textContent=visible?"Hide changes":"Show changes";
    if(visible)await loadShoppingHistory();
  });
  $("#shopping-refresh").addEventListener("click",loadShopping);
  $("#shopping-new-list").addEventListener("click",()=>openShoppingListForm());
  $("#shopping-edit-list").addEventListener("click",()=>{const list=selectedList();if(list?.can_manage)openShoppingListForm(list)});
  $("#shopping-list-cancel").addEventListener("click",()=>$("#shopping-list-form").classList.add("hidden"));
  $("#shopping-list-picker").addEventListener("click",async event=>{
    const button=event.target.closest("[data-shopping-select]");
    if(!button)return;
    state.shopping.listID=button.dataset.shoppingSelect;
    state.shopping.items=[];
    resetShoppingItemForm();
    $("#shopping-category-filter").value="";
    $("#shopping-history-list").classList.add("hidden");
    $("#shopping-history-toggle").setAttribute("aria-expanded","false");
    $("#shopping-history-toggle").textContent="Show changes";
    renderShopping();
    try{await loadShoppingItems()}catch(err){status("#shopping-status",err.message)}
  });
  $("#shopping-list-form").addEventListener("submit",async event=>{
    event.preventDefault();
    const form=event.currentTarget;
    const id=form.elements.namedItem("id").value;
    const name=form.elements.namedItem("name").value.trim();
    const payload={name,shared:form.elements.namedItem("shared").checked,archived:form.elements.namedItem("archived").checked};
    if(!name){status("#shopping-status","Name your shopping list.");return}
    try{
      const result=await api(id?"/api/shopping/lists/"+encodeURIComponent(id):"/api/shopping/lists",
        {method:id?"PUT":"POST",body:JSON.stringify(payload)});
      if(!id)state.shopping.listID=result.id;
      form.classList.add("hidden");
      await loadShopping();
      status("#shopping-status",id?"List updated.":"List created.");
    }catch(err){status("#shopping-status","Couldn't save list: "+err.message)}
  });
  $("#shopping-delete-list").addEventListener("click",async()=>{
    const list=selectedList();if(!list?.can_manage)return;
    if(!confirm('Delete "'+list.name+'" and all its shopping items? This cannot be undone.'))return;
    try{
      await api("/api/shopping/lists/"+encodeURIComponent(list.id),{method:"DELETE"});
      state.shopping.listID=null;
      await loadShopping();
      status("#shopping-status","Shopping list deleted.");
    }catch(err){status("#shopping-status","Couldn't delete list: "+err.message)}
  });
  $("#shopping-item-form").addEventListener("submit",async event=>{
    event.preventDefault();
    const list=selectedList();if(!list||!canShop()||list.archived)return;
    const form=event.currentTarget;
    const id=form.elements.namedItem("id").value;
    const previous=state.shopping.items.find(item=>item.id===id);
    const payload={
      name:form.elements.namedItem("name").value.trim(),
      quantity:Number(form.elements.namedItem("quantity").value),
      unit:form.elements.namedItem("unit").value,
      notes:form.elements.namedItem("notes").value.trim(),
      category:form.elements.namedItem("category").value.trim(),
      assigned_to:form.elements.namedItem("assigned_to").value||null,
      stock_on_hand:form.elements.namedItem("stock_on_hand").value===""?null:Number(form.elements.namedItem("stock_on_hand").value),
      reorder_at:form.elements.namedItem("reorder_at").value===""?null:Number(form.elements.namedItem("reorder_at").value),
      purchased:previous?.purchased||false
    };
    if(payload.reorder_at!==null&&payload.stock_on_hand===null){
      status("#shopping-status","Enter the amount in stock before setting a reorder level.");return;
    }
    try{
      const base="/api/shopping/lists/"+encodeURIComponent(list.id)+"/items";
      await api(id?base+"/"+encodeURIComponent(id):base,{method:id?"PUT":"POST",body:JSON.stringify(payload)});
      resetShoppingItemForm();
      await loadShoppingLists();await loadShoppingItems();await loadShoppingHistory();
      status("#shopping-status",id?"Item saved.":"Item added.");
    }catch(err){status("#shopping-status","Couldn't save item: "+err.message)}
  });
  $("#shopping-item-cancel").addEventListener("click",resetShoppingItemForm);
  $("#shopping-item-list").addEventListener("click",async event=>{
    const edit=event.target.closest("[data-shopping-edit]");
    const del=event.target.closest("[data-shopping-delete]");
    const list=selectedList();if(!list||!canShop()||list.archived)return;
    const id=edit?.dataset.shoppingEdit||del?.dataset.shoppingDelete;
    if(!id)return;
    const item=state.shopping.items.find(x=>x.id===id);if(!item)return;
    if(edit){
      const form=$("#shopping-item-form");
      for(const name of ["name","quantity","unit","notes","category"])form.elements.namedItem(name).value=item[name]??"";
      form.elements.namedItem("assigned_to").value=item.assigned_to||"";
      form.elements.namedItem("stock_on_hand").value=item.stock_on_hand??"";
      form.elements.namedItem("reorder_at").value=item.reorder_at??"";
      form.elements.namedItem("id").value=item.id;
      $("#shopping-item-submit").textContent="Save item";
      $("#shopping-item-cancel").classList.remove("hidden");
      form.elements.namedItem("name").focus();
      return;
    }
    if(!confirm('Remove "'+item.name+'" from this list?'))return;
    try{
      await api("/api/shopping/lists/"+encodeURIComponent(list.id)+"/items/"+encodeURIComponent(id),{method:"DELETE"});
      await loadShoppingLists();await loadShoppingItems();await loadShoppingHistory();
      status("#shopping-status","Item removed.");
    }catch(err){status("#shopping-status","Couldn't remove item: "+err.message)}
  });
  $("#shopping-item-list").addEventListener("change",async event=>{
    const control=event.target.closest("[data-shopping-toggle]");
    if(!control)return;
    const list=selectedList();const item=state.shopping.items.find(x=>x.id===control.dataset.shoppingToggle);
    if(!list||!item||!canShop()||list.archived){renderShopping();return}
    control.disabled=true;
    try{
      await api("/api/shopping/lists/"+encodeURIComponent(list.id)+"/items/"+encodeURIComponent(item.id),
        {method:"PUT",body:JSON.stringify(itemPayload(item,control.checked))});
      await loadShoppingLists();await loadShoppingItems();await loadShoppingHistory();
    }catch(err){status("#shopping-status","Couldn't update item: "+err.message);renderShopping()}
  });
  // Used by existing routing and initial application rendering.
  window.caldenFamily={
    renderDashboard,renderShopping,
    enter(page){
      if(page==="dashboard"){
        renderDashboard();
        if(state.settings?.shopping_enabled!==false)loadShoppingLists().catch(()=>{});
        if(state.settings?.chores_enabled||state.settings?.tasks_enabled)
          loadChores().then(renderDashboard).catch(()=>{});
      }
      if(page==="shopping")loadShopping();
      if(page==="dashboard"&&state.settings?.routines_enabled===true)window.caldenRoutines?.load();
      if(page==="dashboard"&&state.settings?.rewards_enabled===true)window.caldenRewards?.load();
    }
  };
})();
