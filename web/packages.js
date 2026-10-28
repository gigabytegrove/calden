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
  let trackerState="unknown";
  let historyPackageID="";
  let historyOffset=0;
  let historyRequest=0;
  const checking=new Set();
  const trackerPreparing=()=>["pending","installing","retrying"].includes(trackerState);
  const checkTime=v=>v?new Date(v).toLocaleString():"Pending";
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
    // Paid API setup is not part of the normal Calden workflow.
    $("#packages-provider-open").classList.add("hidden");
    $("#packages-add").disabled=!canWrite();
    $("#packages-import-open").disabled=!canWrite();
    if(!enabled()&&state.currentPage==="packages")navigate("calendar",false);
  }
  async function loadTrackerHealth(){
    const box=$("#packages-tracker-health");
    if(!enabled()){trackerState="disabled";box.classList.add("hidden");return}
    try{
      const result=await api("/api/packages/tracker-health");
      trackerState=result.ready===true?"ready":String(result.state||"unavailable");
      box.classList.toggle("hidden",result.ready===true&&result.browser_ready===true);
      box.classList.toggle("package-tracker-warning",trackerState==="retrying"||trackerState==="unavailable");
      box.textContent=result.ready===true&&result.browser_ready===true?"":String(result.message||"Calden is preparing tracking automatically.");
      render();
    }catch(error){
      trackerState="unavailable";
      box.classList.remove("hidden");
      box.classList.add("package-tracker-warning");
      box.textContent="Could not verify the tracking engine: "+error.message;
      render();
    }
  }
  async function load(quiet=false){
    if(!enabled()){shipments=[];render();renderCalendar();return}
    if(loading)return;
    loading=true;
    if(quiet!==true)status("#packages-status","Loading packages…");
    try{
      const res=await api("/api/packages");
      if(!enabled())return;
      shipments=Array.isArray(res?.items)?res.items:[];
      if(quiet!==true)status("#packages-status","");
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
      '<small>'+esc(p.shared?"Shared with household":"Private")+' · '+esc(p.status_source==="manual"?"Manually maintained":p.status_source==="email"?"Imported from email":p.provider_slug?.startsWith("local:")?"Local carrier update":p.provider_slug?.startsWith("public:")?"From public carrier page":p.status_source==="provider"?"Carrier update":"Awaiting first verified carrier scan")+'</small>'+(p.latest_scan?'<small class="package-latest-scan"><strong>Latest carrier activity:</strong> '+esc(p.latest_scan)+'</small>':'<small class="package-latest-scan package-no-scans">No verified carrier scans recorded yet. Open history to review saved changes.</small>')+ '<small>Last checked: '+(p.last_tracking_check?esc(checkTime(p.last_tracking_check)):'Never')+' · Next: '+esc(checkTime(p.next_tracking_check))+'</small>'+(p.tracking_error?'<small class="package-tracking-issue" role="status">Carrier check failed: '+esc(p.tracking_error)+'</small>':'')+'</div></div>'+
      '<div class="package-entry-side"><span class="package-status package-status-'+attr(p.status)+'">'+esc(labelStatus(p.status))+'</span>'+
      '<span class="package-delivery">'+esc(p.expected_date?safeDate(p.expected_date):"Delivery date unknown")+'</span>'+
      (p.can_edit&&p.status!=="delivered"?'<button class="button secondary compact" type="button" data-package-check="'+attr(p.id)+'" '+(checking.has(p.id)||trackerPreparing()?'disabled':'')+'>'+(checking.has(p.id)?'Checking…':trackerPreparing()?'Preparing tracker…':'Check tracking now')+'</button>':'')+
      '<button class="button secondary compact" type="button" data-package-history="'+attr(p.id)+'" aria-label="View full tracking history for '+attr(p.label)+'">Tracking history</button>'+
      (p.can_edit?'<button class="button danger compact" type="button" data-package-delete="'+attr(p.id)+'" aria-label="Delete '+attr(p.label)+' permanently">Delete</button>':'')+
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
  // Tracking history is a direct, read-only package action, not a form field.
  // Keep the actual source of every event visible: a manual entry must never
  // be misrepresented as a carrier-verified scan.
  const historySource=source=>source==="provider"?"Carrier scan":
    source==="provider_status"?"Carrier status":source==="email"?"Shipping email":source==="manual"?"Entered in Calden":"Shipment update";
  const historyDate=raw=>{
    const date=new Date(raw);
    return Number.isNaN(date.getTime())?"Time unavailable":date.toLocaleString();
  };
  function closePackageHistory(){
    historyRequest++;
    historyPackageID="";
    historyOffset=0;
    $("#package-history-dialog").close();
  }
  async function historyFor(id,append=false){
    if(!id||historyPackageID!==id)return;
    const token=++historyRequest;
    const host=$("#package-history");
    const older=$("#package-history-older");
    const feedback=$("#package-history-feedback");
    const refresh=$("#package-history-refresh");
    const offset=append?historyOffset:0;
    older.disabled=true;refresh.disabled=true;
    if(!append){host.innerHTML='<p class="muted">Loading shipment activity…</p>'}
    feedback.textContent=append?"Loading older activity…":"Loading tracking history…";
    try{
      const data=await api("/api/packages/"+encodeURIComponent(id)+"/history?offset="+offset);
      if(historyPackageID!==id||token!==historyRequest||!$("#package-history-dialog").open)return;
      const items=Array.isArray(data?.items)?data.items:[];
      const content=items.map(item=>
        '<li class="package-history-event">'+
          '<span class="package-history-dot" aria-hidden="true"></span>'+
          '<div class="package-history-body">'+
            '<div class="package-history-event-head"><strong>'+esc(labelStatus(item.status))+'</strong>'+
            '<span class="package-history-source">'+esc(historySource(item.source))+'</span></div>'+
            '<p>'+esc(item.detail||"Status recorded")+'</p>'+
            '<time datetime="'+attr(item.occurred_at||"")+'">'+esc(historyDate(item.occurred_at))+'</time>'+
          '</div></li>'
      ).join("");
      if(append){
        const list=host.querySelector(".package-history-timeline");
        if(list)list.insertAdjacentHTML("beforeend",content);
      }else{
        host.innerHTML=items.length?'<ol class="package-history-timeline">'+content+'</ol>':
          '<div class="package-history-empty"><strong>No tracking activity recorded yet.</strong>'+
          '<p>When the carrier provides scans, they will appear here automatically. A saved package does not mean the carrier has confirmed a shipment.</p></div>';
      }
      historyOffset=Number.isSafeInteger(data?.next_offset)?data.next_offset:offset+items.length;
      older.classList.toggle("hidden",data?.has_more!==true);
      feedback.textContent=items.length?
        "Showing "+historyOffset+" recorded "+(historyOffset===1?"event":"events")+
          (data?.has_more?" · older activity available":"")+".": 
        append?"No more activity available.":"";
    }catch(error){
      if(historyPackageID!==id||token!==historyRequest)return;
      if(!append)host.innerHTML='<div class="package-history-empty"><strong>History unavailable</strong>'+
        '<p>The saved history could not be loaded. Try Refresh history.</p></div>';
      feedback.textContent="Could not load tracking history: "+error.message;
      older.classList.toggle("hidden",!append);
    }finally{
      if(historyPackageID===id&&token===historyRequest){
        older.disabled=false;refresh.disabled=false;
      }
    }
  }
  function openPackageHistory(id){
    const p=shipments.find(item=>item.id===id);
    if(!p)return;
    const edit=$("#package-dialog");
    if(edit.open)edit.close();
    const dialog=$("#package-history-dialog");
    if(dialog.open)dialog.close();
    historyPackageID=id;historyOffset=0;historyRequest++;
    $("#package-history-name").textContent=p.label;
    $("#package-history-carrier").textContent=(carriers[p.carrier]||p.carrier)+" · "+p.tracking_number;
    const badge=$("#package-history-current-status");
    badge.className="package-status package-status-"+(statusLabels[p.status]?p.status:"unknown");
    badge.textContent=labelStatus(p.status);
    $("#package-history-check-status").textContent=p.last_tracking_check?
      "Last tracking attempt: "+checkTime(p.last_tracking_check)+(p.tracking_error?". Calden will retry: "+p.tracking_error:""):
      "No carrier lookup completed yet. Calden will check this package automatically.";
    $("#package-history-older").classList.add("hidden");
    $("#package-history-feedback").textContent="";
    dialog.showModal();
    $("#package-history-title").focus();
    historyFor(id);
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
    checkDetail.textContent=p?.last_tracking_check?"Last carrier tracking attempt: "+new Date(p.last_tracking_check).toLocaleString()+". Next scheduled: "+(p.next_tracking_check?new Date(p.next_tracking_check).toLocaleString():"pending")+(p.tracking_error?". "+p.tracking_error:""):"No carrier scan checked yet. Calden attempts automatic tracking every three hours for active packages.";
    const writable=canWrite()&&(!p||p.can_edit);
    form.querySelectorAll("input,select,textarea").forEach(el=>{el.disabled=!writable});
    $("#package-save").classList.toggle("hidden",!writable);
    $("#package-delete").classList.toggle("hidden",!p||!writable);
    $("#package-more-details").open=false;
    $("#package-update-status").classList.toggle("hidden",!p||!writable||p.status==="delivered");
    $("#package-form-heading").textContent=p?(writable?"Edit package":"Package details"):"Add package";
    const link=$("#package-track-link");
    link.classList.toggle("hidden",!p?.tracking_url);
    if(p?.tracking_url){link.href=p.tracking_url}else{link.removeAttribute("href")}
    $("#package-open-history").classList.toggle("hidden",!p);
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
      await load();
      status("#packages-status",(id?"Package updated. ":"Package added. ")+(trackerPreparing()?"Calden will check tracking after its engine is ready.":"Checking carrier tracking in the background…"));
      api("/api/packages/"+encodeURIComponent(id||result.id)+"/refresh",{method:"POST",body:"{}"})
        .then(async outcome=>{
          await load();
          status("#packages-status",outcome.status==="updated"?"Carrier status and available scan history updated.":outcome.status==="repairing"?"Package saved. Calden will check it automatically when the tracker is ready.":outcome.status==="unavailable"?"Tracking is saved; no new carrier scan was returned. Automatic retry is scheduled.":outcome.message||"Carrier tracking scheduled.");
          if(outcome.status==="repairing")loadTrackerHealth();
        })
        .catch(async error=>{
          await load();
          status("#packages-status","Package saved. Carrier scan currently unavailable ("+error.message+"); next automatic check is scheduled.");
          loadTrackerHealth();
        });
    }catch(err){status("#package-form-error",err.message)}
    finally{button.disabled=false}
  }
  async function deletePackage(id){
    const pkg=shipments.find(p=>p.id===id);
    if(!pkg||!pkg.can_edit)return;
    if(!confirm('Permanently delete "'+pkg.label+'"? Its tracking history will be erased and this cannot be undone.'))return;
    const button=$("#package-delete");
    if(button)button.disabled=true;
    status("#packages-status","Deleting "+pkg.label+"…");
    try{
      await api("/api/packages/"+encodeURIComponent(id),{method:"DELETE"});
      const form=$("#package-form");
      if(formValue(form,"id").value===id&&$("#package-dialog").open)$("#package-dialog").close();
      await load();
      status("#packages-status","Package deleted. You can add this tracking number again.");
    }catch(err){
      status("#packages-status","Could not delete package: "+err.message);
      status("#package-form-error","Could not delete package: "+err.message);
    }finally{if(button)button.disabled=false}
  }
  async function loadMailboxSettings(){
    const form=$("#package-mailbox-form");
    const label=$("#package-mailbox-status");
    label.textContent="Checking mailbox connection…";
    const result=await api("/api/packages/mailbox");
    const isGmail=!result.hostname||result.hostname==="imap.gmail.com";
    $("#package-email-provider").value=isGmail?"gmail":"other";
    form.elements.namedItem("hostname").value=isGmail?"auto":result.hostname;
    $("#package-mailbox-advanced").open=!isGmail;
    updateEmailProvider(false);
    form.elements.namedItem("username").value=result.username||"";
    form.elements.namedItem("password").value="";
    form.elements.namedItem("enabled").checked=result.configured?Boolean(result.enabled):true;
    label.textContent=!result.configured?"No mailbox connected. Enter a username and app password to start.":(
      (result.enabled?"Automatic inbox checks enabled. ":"Inbox checks paused. ")+
      (result.last_scan?"Last scan: "+new Date(result.last_scan).toLocaleString()+". ":"First scan pending. ")+
      (result.last_error?"Last error: "+result.last_error+". ":"")+
      (result.last_scan?"Messages examined: "+Number(result.last_imported||0)+".":"")
    );
  }
  function updateEmailProvider(resetHost=true){
    const gmail=$("#package-email-provider").value==="gmail";
    const form=$("#package-mailbox-form");
    $("#package-mailbox-advanced").open=!gmail;
    if(resetHost)form.elements.namedItem("hostname").value=gmail?"auto":"";
    const help=$("#package-email-account-hint");
    help.textContent=gmail
      ?"Calden chooses Gmail's secure server automatically. Use a Google app password, not your account password."
      :"Ask your Calden administrator which mail server is approved. Microsoft 365 and Outlook require OAuth and are not supported yet.";
    $("#package-mailbox-password-help").classList.toggle("hidden",!gmail);
    form.elements.namedItem("password").placeholder=gmail?"Paste a Google app password":"Enter the mailbox app password";
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
      status("#package-mailbox-status","Inbox settings saved. Calden will verify the connection on its next scan and check automatically every 15 minutes.");
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
      ?"Manual carrier override: "+(carriers[selected]||selected)+". Calden checks available public carrier sources."
      :guess==="other"?"Carrier cannot be confidently determined from this format. Calden will attempt public carrier detection."
      :"Likely "+carriers[guess]+" (number format only). Calden will attempt to verify this.";
    status("#package-carrier-suggestion",message);
  }
  async function checkPackage(id){
    const p=shipments.find(item=>item.id===id);
    if(!p||!p.can_edit||p.status==="delivered"||checking.has(id))return;
    checking.add(id);render();
    status("#packages-status","Checking carrier tracking for "+(p.label||p.tracking_number)+"…");
    try{
      const outcome=await api("/api/packages/"+encodeURIComponent(id)+"/refresh",{method:"POST",body:"{}"});
      await load();
      status("#packages-status",outcome.status==="updated"?"Carrier status and history refreshed.":outcome.message||"No new verified carrier scan available.");
      if(outcome.status==="unavailable"||outcome.status==="repairing")loadTrackerHealth();
    }catch(error){
      await load();
      loadTrackerHealth();
      status("#packages-status","Carrier check unsuccessful: "+error.message);
    }finally{checking.delete(id);render()}
  }
  async function refreshCurrentPackage(){
    const id=formValue($("#package-form"),"id").value;
    if(!id)return;
    const button=$("#package-update-status");button.disabled=true;
    status("#package-tracking-details","Retrieving the latest carrier scans…");
    try{
      const result=await api("/api/packages/"+encodeURIComponent(id)+"/refresh",{method:"POST",body:"{}"});
      await load();
      const updated=shipments.find(item=>item.id===id);
      if(updated)openPackage(id);
      status("#package-tracking-details",result.status==="updated"?"Status retrieved from carrier tracking.":result.message);
      if(result.status==="unavailable"||result.status==="repairing")loadTrackerHealth();
    }catch(e){status("#package-tracking-details","Could not refresh carrier data: "+e.message)}
    finally{button.disabled=false}
  }
  async function loadTrackingProvider(){
    const data=await api("/api/packages/provider");
    const form=$("#package-provider-form");
    form.elements.namedItem("api_key").value="";
    form.elements.namedItem("enabled").checked=Boolean(data.enabled);
    status("#package-provider-status",data.configured?(data.enabled?"API key configured (carrier access not yet verified). Active packages are checked every three hours.":"API key saved; automatic checks paused."):"No API key required. Calden includes a built-in local tracking engine. AfterShip is optional and not needed for normal operation.");
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
    if(enabled())loadTrackerHealth();
    if(enabled()){load()}else{trackerState="disabled";shipments=[];render();renderCalendar()}
  }
  function enter(page){
    if(page==="packages"&&enabled()){load();loadTrackerHealth()}
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
    const check=event.target.closest("[data-package-check]");
    if(check){checkPackage(check.dataset.packageCheck);return}
    const history=event.target.closest("[data-package-history]");
    if(history){openPackageHistory(history.dataset.packageHistory);return}
    const remove=event.target.closest("[data-package-delete]");
    if(remove){deletePackage(remove.dataset.packageDelete);return}
    const button=event.target.closest("[data-package-open]");
    if(button)openPackage(button.dataset.packageOpen);
  });
  $("#package-form").addEventListener("submit",save);
  $("#package-delete").addEventListener("click",()=>deletePackage(formValue($("#package-form"),"id").value));
  $("#package-open-history").addEventListener("click",()=>openPackageHistory(formValue($("#package-form"),"id").value));
  $("#package-history-close").addEventListener("click",closePackageHistory);
  $("#package-history-refresh").addEventListener("click",()=>historyFor(historyPackageID));
  $("#package-history-older").addEventListener("click",()=>historyFor(historyPackageID,true));
  $("#package-history-dialog").addEventListener("close",()=>{
    historyRequest++;historyPackageID="";historyOffset=0;
  });
  document.querySelectorAll("[data-close-package]").forEach(b=>b.addEventListener("click",()=>$("#package-dialog").close()));
  document.querySelectorAll("[data-close-package-email]").forEach(b=>b.addEventListener("click",()=>$("#package-email-dialog").close()));
  $("#package-email-dialog").addEventListener("close",()=>{$("#package-email-token-value").value=""});
  $("#package-email-form").addEventListener("submit",importEmail);
  $("#package-token-generate").addEventListener("click",generateToken);
  $("#package-token-revoke").addEventListener("click",revokeToken);
  $("#package-email-provider").addEventListener("change",()=>updateEmailProvider());
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
  // Carrier/repair progress is an actual background workflow. Refresh the
  // visible package statuses and readiness automatically without user clicks.
  window.setInterval(()=>{
    if(enabled()&&state.currentPage==="packages"){
      if(!loading)load(true);
      loadTrackerHealth();
      if(historyPackageID&&$("#package-history-dialog").open&&$("#package-history-older").classList.contains("hidden")){
        // Refresh newest events only while the timeline is on its first page.
        historyFor(historyPackageID);
      }
    }
  },30000);
  window.caldenPackages={load,render,renderNav,enter,onToggle,calendarForDay,bindCalendar};
})();
