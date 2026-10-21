;(function caldenPersonalNotifications(){
 const form=document.getElementById("personal-notifications-form");
 const status=document.getElementById("personal-notifications-status");
 const get=name=>form.elements.namedItem(name);
 const fields=form.querySelector("[data-quiet-fields]");
 function toggleQuiet(){
  const enabled=get("quiet_enabled").checked;
  fields.classList.toggle("hidden",!enabled);
  get("quiet_start").disabled=!enabled;
  get("quiet_end").disabled=!enabled;
 }
 get("quiet_enabled").addEventListener("change",toggleQuiet);
 toggleQuiet();
 async function load(){
  if(state.me?.is_child && !(state.me.child_modules||[]).includes("notifications"))return;
  try{
   const cfg=await api("/api/me/notification-preferences");
   get("live_enabled").checked=!!cfg.live_enabled;
   get("push_enabled").checked=!!cfg.push_enabled;
   const kinds=new Set(cfg.enabled_kinds||[]);
   form.querySelectorAll('input[name="kind"]').forEach(box=>box.checked=kinds.has(box.value));
   get("quiet_enabled").checked=!!cfg.quiet_start&&!!cfg.quiet_end;
   get("quiet_start").value=cfg.quiet_start||"22:00";
   get("quiet_end").value=cfg.quiet_end||"07:00";
   toggleQuiet();status.textContent="";
  }catch(err){status.textContent="Couldn't load your reminder settings: "+err.message}
 }
 form.addEventListener("submit",async event=>{
  event.preventDefault();
  const quiet=get("quiet_enabled").checked;
  if(quiet&&get("quiet_start").value===get("quiet_end").value){
   status.textContent="Choose different start and end times for quiet hours.";return;
  }
  const prefs={
   push_enabled:get("push_enabled").checked,
   live_enabled:get("live_enabled").checked,
   enabled_kinds:[...form.querySelectorAll('input[name="kind"]:checked')].map(x=>x.value),
   quiet_start:quiet?get("quiet_start").value:null,
   quiet_end:quiet?get("quiet_end").value:null
  };
  const submit=form.querySelector('[type="submit"]');submit.disabled=true;
  try{
   await api("/api/me/notification-preferences",{method:"PUT",body:JSON.stringify(prefs)});
   status.textContent="Your reminder preferences are saved across devices.";
  }catch(err){status.textContent="Couldn't save your reminder preferences: "+err.message}
  finally{submit.disabled=false}
 });
 window.caldenPersonalNotifications={load};
})();
