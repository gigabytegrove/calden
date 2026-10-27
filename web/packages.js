"use strict";
;(() => {
  // Calden package tracking deliberately uses a separate module and database.
  // Calendar cells are projections of shipment dates, not duplicate event rows.
  const $=selector=>document.querySelector(selector);
  const esc=value=>escapeHTML(String(value??""));
  const attr=value=>escapeAttr(String(value??""));
  const statusLabels={
    unknown:"Not yet verified",label_created:"Label created",in_transit:"In transit",
    out_for_delivery:"Out for delivery",delivered:"Delivered",exception:"Delivery exception"
  };
  const carriers={auto:"Auto-detect",amazon:"Amazon",usps:"USPS",ups:"UPS",fedex:"FedEx",other:"Other"};
  let shipments=[];
  let loading=false;
  let visible=localStorage.getItem("calden_packages_calendar")!=="false";
  const enabled=()=>state.settings?.packages_enabled===true&&!state.me?.is_child;
  const canWrite=()=>state.me?.role!=="restricted"&&!state.me?.is_child;
  const formValue=(form,name)=>form.elements.namedItem(name);
  const labelStatus=value=>statusLabels[value]||statusLabels.unknown;
  const status=(id,message)=>{const el=$(id);if(el)el.textContent=message||""};
  const safeDate=value=>{
    if(!value||!/^\d{4}-\d{2}-\d{2}$/.test(value))return "Date not provided";
    const [year,month,day]=value.split("-").map(Number);
    const result=new Date(year,month-1,day);
    if(result.getFullYear()!==year||result.getMonth()!==month-1||result.getDate()!==day)return "Date not provided";
    return result.toLocaleDateString(undefined,{weekday:"short",month:"short",day:"numeric",year:"numeric"});
  };
  function renderNav(){
    document.querySelectorAll(".packages-nav").forEach(el=>el.classList.toggle("hidden",!enabled()));
    $("#packages-calendar-toggle")?.classList.toggle("hidden",!enabled());
    if($("#packages-calendar-visible"))$("#packages-calendar-visible").checked=visible;
    $("#packages-provider-open").classList.toggle("hidden",state.me?.role!=="admin");
    $("#packages-add").disabled=!canWrite();
    $("#packages-import-open").disabled=!canWrite();
    if(!enabled()&&state.currentPage==="packages")navigate("calendar",false);
  }
  async function load(){
    if(!enabled()){shipments=[];render();renderCalendar();return}
    if(loading)return;
    loading=true;
    status("#packages-status","Loading packages…");
    try{
      const res=await api("/api/packages");
      if(!enabled())return;
      shipments=Array.isArray(res?.items)?res.items:[];
      status("#packages-status","");
      render();
      renderCalendar();
    }catch(err){
      if(err?.status===403){shipments=[]}
      status("#packages-status","Couldn't load packages: "+err.message);
      render();
      renderCalendar();
    }finally{loading=false}
  }
  function render(){
    const active=shipments.filter(p=>p.status!=="delivered");
    const delivered=shipments.filter(p=>p.status==="delivered");
    const today=dateInputValue(new Date());
    const due=active.filter(p=>p.expected_date===today);
    $("#packages-summary").innerHTML=[
      [active.length,"Incoming"],[due.length,"Expected today"],[delivered.length,"Delivered"]
    ].map(([n,label])=>'<div class="package-total"><strong>'+n+'</strong><span>'+label+'</span></div>').join("");
    const filter=$("#packages-filter").value;
    const query=$("#packages-search").value.trim().toLowerCase();
    const shown=shipments.filter(p=>
      (filter==="all"||(filter==="active"&&p.status!=="delivered")||(filter==="delivered"&&p.status==="delivered"))&&
      (!query||[p.label,p.carrier,p.tracking_number,p.status].some(text=>String(text||"").toLowerCase().includes(query)))
    );
    const host=$("#packages-list");
    if(!shown.length){
      host.innerHTML='<div class="package-empty"><span aria-hidden="true">📦</span><h3>No packages here</h3><p>'+(
        shipments.length?"Try another filter or search.":"Add a tracking number or import a shipping email to get started."
      )+'</p></div>';
      return;
    }
    host.innerHTML=shown.map(p=>'<article class="package-entry"><div class="package-entry-main">'+
      '<div class="package-entry-icon" aria-hidden="true">📦</div><div class="package-entry-description">'+
      '<strong>'+esc(p.label)+'</strong><span>'+esc(carriers[p.carrier]||p.carrier)+' · '+esc(p.tracking_number)+'</span>'+
      '<small>'+esc(p.shared?"Shared with household":"Private")+' · '+esc(p.status_source==="manual"?"Manually maintained":p.status_source==="email"?"Imported from email":p.provider_slug?.startsWith("public:")?"From public carrier page":"Carrier API update")+'</small>'+ (p.tracking_error?'<small class="package-tracking-issue">Tracking check: '+esc(p.tracking_error)+'</small>':'')+'</div></div>'+
      '<div class="package-entry-side"><span class="package-status package-status-'+attr(p.status)+'">'+esc(labelStatus(p.status))+'</span>'+
      '<span class="package-delivery">'+esc(p.expected_date?safeDate(p.expected_date):"Delivery date unknown")+'</span>'+
      '<button class="button secondary compact" type="button" data-package-open="'+attr(p.id)+'">'+(p.can_edit?"Open / edit":"View")+'</button></div></article>'
    ).join("");
  }
  function calendarForDay(day){
    if(!enabled()||!visible)return "";
    const target=dateInputValue(day);
    return shipments.filter(p=>p.expected_date===target&&p.status!=="delivered").map(p=>
      '<button type="button" class="calendar-event compact package-calendar-item" style="--cal:#a36c24" data-package-id="'+attr(p.id)+'" title="'+attr("Package: "+p.label+" — "+labelStatus(p.status))+'">'+
      '<span class="event-color"></span><span class="calendar-event-copy"><strong>📦 '+esc(p.label)+'</strong><small>'+esc(labelStatus(p.status))+'</small></span></button>'
    ).join("");
  }
  function bindCalendar(host){
    if(host.dataset.packageEventsBound)return;
    host.dataset.packageEventsBound="true";
    host.addEventListener("click",event=>{
      const button=event.target.closest("[data-package-id]");
      if(!button)return;
      event.stopPropagation();
      const packageID=button.dataset.packageId;
      if(shipments.some(p=>p.id===packageID))openPackage(packageID);
    });
  }
  async function historyFor(id){
    const host=$("#package-history");
    host.innerHTML='<p class="muted">Loading status history…</p>';
    try{
      const data=await api("/api/packages/"+encodeURIComponent(id)+"/history");
      const items=Array.isArray(data?.items)?data.items:[];
      host.innerHTML=items.length?'<ol class="package-history">'+items.map(item=>
        '<li><strong>'+esc(labelStatus(item.status))+'</strong><span>'+esc(item.detail)+'</span>'+
        '<small>'+esc(new Date(item.occurred_at).toLocaleString())+' · '+esc(item.source)+'</small></li>'
      ).join("")+'</ol>':'<p class="muted">No status changes yet.</p>';
    }catch(err){host.textContent="Could not load history: "+err.message}
  }
  function openPackage(id=""){
    const p=shipments.find(item=>item.id===id);
    const form=$("#package-form");
    form.reset();
    status("#package-form-error","");
    formValue(form,"id").value=p?.id||"";
    for(const key of ["label","carrier","tracking_number","status","expected_date","notes"]){
      formValue(form,key).value=p?.[key]??(key==="carrier"?"auto":key==="status"?"unknown":"");
    }
    formValue(form,"shared").checked=!!p?.shared;
    updateCarrierSuggestion();
    const checkDetail=$("#package-tracking-details");
    checkDetail.textContent=p?.last_tracking_check?"Last public tracking attempt: "+new Date(p.last_tracking_check).toLocaleString()+". Next scheduled: "+(p.next_tracking_check?new Date(p.next_tracking_check).toLocaleString():"pending")+(p.tracking_error?". "+p.tracking_error:""):"Not checked yet. Calden automatically attempts public tracking every three hours while this package is active.";
    const writable=canWrite()&&(!p||p.can_edit);
    form.querySelectorAll("input,select,textarea").forEach(el=>{el.disabled=!writable});
    $("#package-save").classList.toggle("hidden",!writable);
    $("#package-archive").classList.toggle("hidden",!p||!writable);
    $("#package-update-status").classList.toggle("hidden",!p||!writable||p.status==="delivered");
    $("#package-form-heading").textContent=p?(writable?"Edit package":"Package details"):"Add package";
    const link=$("#package-track-link");
    link.classList.toggle("hidden",!p?.tracking_url);
    if(p?.tracking_url){link.href=p.tracking_url}else{link.removeAttribute("href")}
    $("#package-history-panel").classList.toggle("hidden",!p);
    if(p)historyFor(p.id);
    $("#package-dialog").showModal();
    if(writable)formValue(form,"label").focus();
  }
  async function save(event){
    event.preventDefault();
    const form=$("#package-form"),id=formValue(form,"id").value;
    const button=$("#package-save");
    const body={};
    for(const key of ["label","carrier","tracking_number","status","expected_date","notes"])body[key]=formValue(form,key).value.trim();
    body.shared=formValue(form,"shared").checked;
    button.disabled=true;
    status("#package-form-error","");
    try{
      const result=await api(id?"/api/packages/"+encodeURIComponent(id):"/api/packages",{
        method:id?"PUT":"POST",body:JSON.stringify(body)
      });
      $("#package-dialog").close();
      let lookup="Calden will attempt public tracking automatically every three hours.";
      try{
        const outcome=await api("/api/packages/"+encodeURIComponent(id||result.id)+"/refresh",{method:"POST",body:"{}"});
        lookup=outcome.status==="updated"?"Public carrier status retrieved.":outcome.status==="unavailable"?outcome.message:"Tracking checked or already scheduled for its next three-hour refresh.";
      }catch(error){
        lookup="Automatic tracking not yet available: "+error.message;
      }
      await load();
      status("#packages-status",(id?"Package updated. ":"Package added. ")+lookup);
    }catch(err){status("#package-form-error",err.message)}
    finally{button.disabled=false}
  }
  async function archive(){
    const id=formValue($("#package-form"),"id").value;
    if(!id||!confirm("Archive this package? It will disappear from the calendar and package list, but its record will be retained."))return;
    const b=$("#package-archive");b.disabled=true;
    try{
      await api("/api/packages/"+encodeURIComponent(id),{method:"DELETE"});
      $("#package-dialog").close();await load();
      status("#packages-status","Package archived.");
    }catch(err){status("#package-form-error",err.message)}
    finally{b.disabled=false}
  }
  async function loadMailboxSettings(){
    const form=$("#package-mailbox-form");
    const label=$("#package-mailbox-status");
    label.textContent="Checking mailbox connection…";
    const result=await api("/api/packages/mailbox");
    form.elements.namedItem("hostname").value=result.hostname||"imap.gmail.com";
    form.elements.namedItem("username").value=result.username||"";
    form.elements.namedItem("password").value="";
    form.elements.namedItem("enabled").checked=Boolean(result.enabled);
    label.textContent=!result.configured?"No mailbox connected. Enter a username and app password to start.":(
      (result.enabled?"Automatic inbox checks enabled. ":"Inbox checks paused. ")+
      (result.last_scan?"Last scan: "+new Date(result.last_scan).toLocaleString()+". ":"First scan pending. ")+
      (result.last_error?"Last error: "+result.last_error+". ":"")+
      (result.last_scan?"Messages examined: "+Number(result.last_imported||0)+".":"")
    );
  }
  async function saveMailboxSettings(event){
    event.preventDefault();
    const form=$("#package-mailbox-form"),button=$("#package-mailbox-save");
    const fields=Object.fromEntries(new FormData(form).entries());
    fields.enabled=form.elements.namedItem("enabled").checked;
    button.disabled=true;
    status("#package-mailbox-status","Saving encrypted mailbox connection…");
    try{
      await api("/api/packages/mailbox",{method:"PUT",body:JSON.stringify(fields)});
      form.elements.namedItem("password").value="";
      await loadMailboxSettings();
      status("#package-mailbox-status","Mailbox saved. Enabled connections are checked shortly and then about every 15 minutes.");
    }catch(err){status("#package-mailbox-status",err.message)}
    finally{button.disabled=false}
  }
  async function requestMailboxScan(){
    const button=$("#package-mailbox-check");button.disabled=true;
    try{
      await api("/api/packages/mailbox/check",{method:"POST",body:"{}"});
      status("#package-mailbox-status","Mailbox check queued. Refresh this status after the server finishes scanning.");
    }catch(err){status("#package-mailbox-status",err.message)}
    finally{button.disabled=false}
  }
  async function disconnectMailbox(){
    if(!confirm("Disconnect shipping email discovery? Saved mailbox credentials and scan state will be deleted. Existing packages are retained."))return;
    try{
      await api("/api/packages/mailbox",{method:"DELETE"});
      await loadMailboxSettings();
      status("#package-mailbox-status","Mailbox disconnected. Existing packages were not deleted.");
    }catch(err){status("#package-mailbox-status",err.message)}
  }
  function openEmail(){
    $("#package-email-form").reset();
    $("#package-email-token-value").value="";
    $("#package-inbound-url").textContent=location.origin+"/api/packages/inbound-email";
    status("#package-email-status","");
    status("#package-token-status","Checking forwarding token…");
    $("#package-email-dialog").showModal();
    loadMailboxSettings().catch(err=>status("#package-mailbox-status",err.message));
    api("/api/packages/email-token").then(data=>{
      status("#package-token-status",data.configured?"A forwarding token is configured. Rotate it to see a new value.":"No forwarding token configured.");
    }).catch(err=>status("#package-token-status",err.message));
  }
  async function importEmail(event){
    event.preventDefault();
    const form=$("#package-email-form"),file=$("#package-email-file").files?.[0];
    let raw=form.elements.namedItem("raw_email").value;
    if(file){
      if(file.size>262144){status("#package-email-status","Email file exceeds 256 KB.");return}
      raw=await file.text();
    }
    if(!raw.trim()){status("#package-email-status","Paste an email or select an .eml file.");return}
    const button=form.querySelector('button[type="submit"]');
    button.disabled=true;
    status("#package-email-status","Extracting tracking numbers…");
    try{
      const result=await api("/api/packages/import-email",{method:"POST",body:JSON.stringify({raw_email:raw})});
      status("#package-email-status",result.created+" package(s) imported; "+result.skipped+" already tracked. Imported emails are not carrier-verified.");
      await load();
    }catch(err){status("#package-email-status",err.message)}
    finally{button.disabled=false}
  }
  async function generateToken(){
    if(!confirm("Generate a new import token? Any existing mail-gateway token will immediately stop working."))return;
    try{
      const data=await api("/api/packages/email-token",{method:"POST",body:"{}"});
      $("#package-email-token-value").value=data.token;
      status("#package-token-status","Copy this new token into your trusted mail gateway now. Calden cannot display it again.");
    }catch(err){status("#package-token-status",err.message)}
  }
  async function revokeToken(){
    if(!confirm("Revoke email forwarding access? Messages sent using the current token will be rejected."))return;
    try{
      await api("/api/packages/email-token",{method:"DELETE"});
      $("#package-email-token-value").value="";
      status("#package-token-status","Email forwarding token revoked.");
    }catch(err){status("#package-token-status",err.message)}
  }
  function detectCarrier(number){
    const n=String(number||"").trim().toUpperCase().replace(/\s+/g,"");
    if(/^1Z[0-9A-Z]{16}$/.test(n))return "ups";
    if(/^(TBA|TBM|TBC)[0-9]{12,15}$/.test(n))return "amazon";
    if(/^(9[2345][0-9]{18,22}|[A-Z]{2}[0-9]{9}US)$/.test(n))return "usps";
    if(/^([0-9]{12}|[0-9]{15})$/.test(n))return "fedex";
    return "other";
  }
  function updateCarrierSuggestion(){
    const f=$("#package-form"),number=formValue(f,"tracking_number").value;
    const selected=formValue(f,"carrier").value,guess=detectCarrier(number);
    const message=selected!=="auto"
      ?"Manual carrier override: "+(carriers[selected]||selected)+". Live API results still require a connected provider."
      :guess==="other"?"Carrier cannot be confidently determined from this format. An authorized tracking API will attempt detection."
      :"Likely "+carriers[guess]+" (number format only). The tracking API will verify this.";
    status("#package-carrier-suggestion",message);
  }
  async function refreshCurrentPackage(){
    const id=formValue($("#package-form"),"id").value;
    if(!id)return;
    const button=$("#package-update-status");button.disabled=true;
    status("#package-tracking-details","Checking the carrier’s public tracking page…");
    try{
      const result=await api("/api/packages/"+encodeURIComponent(id)+"/refresh",{method:"POST",body:"{}"});
      await load();
      const updated=shipments.find(item=>item.id===id);
      if(updated)openPackage(id);
      status("#package-tracking-details",result.status==="updated"?"Status retrieved from carrier tracking.":result.message);
    }catch(e){status("#package-tracking-details","Could not refresh carrier data: "+e.message)}
    finally{button.disabled=false}
  }
  async function loadTrackingProvider(){
    const data=await api("/api/packages/provider");
    const form=$("#package-provider-form");
    form.elements.namedItem("api_key").value="";
    form.elements.namedItem("enabled").checked=Boolean(data.enabled);
    status("#package-provider-status",data.configured?(data.enabled?"API key configured (carrier access not yet verified). Active packages are checked every three hours.":"API key saved; automatic checks paused."):"No API key configured. Calden will use public carrier tracking pages automatically; optional API integration is only for additional coverage.");
  }
  async function saveTrackingProvider(event){
    event.preventDefault();
    const f=$("#package-provider-form"),btn=$("#package-provider-save");
    const body={api_key:f.elements.namedItem("api_key").value,enabled:f.elements.namedItem("enabled").checked};
    btn.disabled=true;
    status("#package-provider-status","Saving encrypted provider settings…");
    try{
      await api("/api/packages/provider",{method:"PUT",body:JSON.stringify(body)});
      await loadTrackingProvider();
      status("#package-provider-status","Settings saved. Check an active package to verify that your API credentials and plan are accepted.");
    }catch(e){status("#package-provider-status",e.message)}
    finally{btn.disabled=false}
  }
  async function disconnectTrackingProvider(){
    if(!confirm("Remove the optional API connection? Keyless public tracking attempts will continue. Saved package history will remain."))return;
    try{
      await api("/api/packages/provider",{method:"DELETE"});
      await loadTrackingProvider();
      status("#package-provider-status","Optional API disconnected. Public tracking attempts continue every three hours.");
    }catch(e){status("#package-provider-status",e.message)}
  }
  function onToggle(){
    renderNav();
    if(enabled()){load()}else{shipments=[];render();renderCalendar()}
  }
  function enter(page){
    if(page==="packages"&&enabled())load();
  }
  $("#packages-refresh").addEventListener("click",load);
  $("#packages-add").addEventListener("click",()=>openPackage());
  $("#packages-import-open").addEventListener("click",openEmail);
  $("#packages-provider-open").addEventListener("click",()=>{
    $("#package-provider-dialog").showModal();
    loadTrackingProvider().catch(e=>status("#package-provider-status",e.message));
  });
  $("#package-provider-close").addEventListener("click",()=>$("#package-provider-dialog").close());
  $("#package-provider-form").addEventListener("submit",saveTrackingProvider);
  $("#package-provider-remove").addEventListener("click",disconnectTrackingProvider);
  $("#package-update-status").addEventListener("click",refreshCurrentPackage);
  $("#package-form").elements.namedItem("tracking_number").addEventListener("input",updateCarrierSuggestion);
  $("#package-form").elements.namedItem("carrier").addEventListener("change",updateCarrierSuggestion);
  $("#packages-filter").addEventListener("change",render);
  $("#packages-search").addEventListener("input",render);
  $("#packages-list").addEventListener("click",event=>{
    const button=event.target.closest("[data-package-open]");
    if(button)openPackage(button.dataset.packageOpen);
  });
  $("#package-form").addEventListener("submit",save);
  $("#package-archive").addEventListener("click",archive);
  document.querySelectorAll("[data-close-package]").forEach(b=>b.addEventListener("click",()=>$("#package-dialog").close()));
  document.querySelectorAll("[data-close-package-email]").forEach(b=>b.addEventListener("click",()=>$("#package-email-dialog").close()));
  $("#package-email-dialog").addEventListener("close",()=>{$("#package-email-token-value").value=""});
  $("#package-email-form").addEventListener("submit",importEmail);
  $("#package-token-generate").addEventListener("click",generateToken);
  $("#package-token-revoke").addEventListener("click",revokeToken);
  $("#package-mailbox-form").addEventListener("submit",saveMailboxSettings);
  $("#package-mailbox-check").addEventListener("click",requestMailboxScan);
  $("#package-mailbox-reload").addEventListener("click",()=>loadMailboxSettings().catch(err=>status("#package-mailbox-status",err.message)));
  $("#package-mailbox-remove").addEventListener("click",disconnectMailbox);
  $("#packages-calendar-visible").checked=visible;
  $("#packages-calendar-visible").addEventListener("change",event=>{
    visible=event.target.checked;
    localStorage.setItem("calden_packages_calendar",String(visible));
    renderCalendar();
  });
  window.caldenPackages={load,render,renderNav,enter,onToggle,calendarForDay,bindCalendar};
})();
