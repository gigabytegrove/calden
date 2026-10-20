;(function caldenRoutineClient(){
 const form=$("#routine-form"),stepsBox=$("#routine-edit-steps"),date=$("#routine-day");
 const field=n=>form.elements.namedItem(n);
 const manager=()=>["admin","planner"].includes(state.me?.role);
 const status=v=>$("#routine-status").textContent=v||"";
 const today=()=>dateInputValue(new Date());
 const activeDate=()=>state.routineDate||today();
 const stepRow=step=>{
   const row=document.createElement("div");
   row.className="routine-edit-row";row.dataset.stepId=step?.id||"";
   row.innerHTML=`<input type="text" maxlength="180" required placeholder="Checklist step" aria-label="Checklist step" value="${escapeAttr(step?.title||"")}">
    <button type="button" class="button secondary compact" data-step-move="-1" aria-label="Move up">↑</button>
    <button type="button" class="button secondary compact" data-step-move="1" aria-label="Move down">↓</button>
    <button type="button" class="button secondary compact" data-step-remove aria-label="Remove">×</button>`;
   return row;
 };
 function edit(item=null){
   if(!manager())return;
   form.reset();field("id").value=item?.id||"";field("title").value=item?.title||"";
   field("notes").value=item?.notes||"";
   field("assigned_to").innerHTML=state.users.filter(u=>u.active!==false)
     .map(u=>`<option value="${escapeAttr(u.id)}">${escapeHTML(u.display_name)}</option>`).join("");
   field("assigned_to").value=item?.assigned_to||state.me.id;
   field("time_of_day").value=item?.time_of_day||"morning";
   const days=new Set(item?.days_of_week||[1,2,3,4,5,6,7]);
   form.querySelectorAll("[name=routine_day]").forEach(x=>x.checked=days.has(Number(x.value)));
   stepsBox.replaceChildren(...(item?.steps||[{title:""}]).map(stepRow));
   $("#routine-form-heading").textContent=item?"Edit routine":"New routine";
   form.classList.remove("hidden");field("title").focus();
 }
 function render(){
   if(!state.me)return;
   const enabled=state.settings?.routines_enabled===true;
   $$(".routines-nav").forEach(el=>el.classList.toggle("hidden",!enabled));
   $("#routine-new").classList.toggle("hidden",!manager());
   date.value=activeDate();
   const host=$("#routine-list");
   if(!enabled){host.innerHTML="";return}
   const records=state.routines||[];
   const due=records.filter(r=>r.due_today);
   const complete=due.reduce((n,r)=>n+r.done,0),all=due.reduce((n,r)=>n+r.total,0);
   $("#routine-progress-summary").textContent=`${complete} of ${all} steps done`;
   if(!records.length){
     host.innerHTML=`<div class="panel"><h3>No routines yet</h3><p class="muted">${manager()?"Add the first checklist for someone in your household.":"Ask your household administrator to assign a routine."}</p></div>`;
     return;
   }
   const parts={morning:"Morning",evening:"Evening",anytime:"Anytime"};
   host.innerHTML=records.map(r=>{
     const pct=r.total?Math.round(r.done/r.total*100):0;
     const controls=r.can_manage?`<div class="form-actions">
       <button type="button" class="button secondary compact" data-routine-edit="${escapeAttr(r.id)}">Edit</button>
       <button type="button" class="button secondary compact" data-routine-archive="${escapeAttr(r.id)}">Archive</button></div>`:"";
     const checklist=r.due_today?`<div class="routine-meter" role="progressbar" aria-valuemin="0" aria-valuemax="100" aria-valuenow="${pct}" aria-label="Routine completion"><span style="width:${pct}%"></span></div>
       <p class="muted">${r.done} of ${r.total} steps finished</p>
       <div class="routine-step-list">${r.steps.map(step=>`<label class="routine-step ${step.completed?"is-completed":""}">
         <input type="checkbox" data-routine-check="${escapeAttr(r.id)}" data-step-id="${escapeAttr(step.id)}"
          ${step.completed?"checked":""} ${!r.can_complete?"disabled":""}>
         <span>${escapeHTML(step.title)}</span></label>`).join("")}</div>`
       :'<p class="muted">Not scheduled for this date.</p>';
     return `<article class="panel routine-card" data-routine-id="${escapeAttr(r.id)}">
       <div class="family-sidebar-heading"><div><p class="eyebrow">${escapeHTML(parts[r.time_of_day]||"Routine")}</p>
       <h3>${escapeHTML(r.title)}</h3><p class="muted">${escapeHTML(r.assignee_name)}${r.notes?" · "+escapeHTML(r.notes):""}</p></div>${controls}</div>
       ${checklist}</article>`;
   }).join("");
 }
 async function load(){
   if(state.settings?.routines_enabled!==true)return;
   const selected=activeDate();
   try{
     const body=await api("/api/routines?date="+encodeURIComponent(selected));
     if(activeDate()!==selected)return;
     state.routines=body.items||[];status("");render();
     window.caldenFamily?.renderDashboard();
   }catch(err){status("Couldn't load routines: "+err.message)}
 }
 function navigateDate(){
   if(state.currentPage==="routines")history.replaceState({calden:true},"","/routines?date="+encodeURIComponent(activeDate()));
 }
 $("#routine-new").addEventListener("click",()=>edit());
 $("#routine-cancel").addEventListener("click",()=>form.classList.add("hidden"));
 $("#routine-refresh").addEventListener("click",load);
 $("#routine-today").addEventListener("click",()=>{
   state.routineDate=today();navigateDate();load();
 });
 date.addEventListener("change",()=>{
   if(!/^\d{4}-\d{2}-\d{2}$/.test(date.value))return;
   state.routineDate=date.value;navigateDate();load();
 });
 $("#routine-add-step").addEventListener("click",()=>{
   if(stepsBox.children.length>=30){status("A routine can have up to 30 steps.");return}
   const row=stepRow();stepsBox.append(row);row.querySelector("input").focus();
 });
 stepsBox.addEventListener("click",event=>{
   const row=event.target.closest(".routine-edit-row");
   if(!row)return;
   if(event.target.closest("[data-step-remove]")){
     if(stepsBox.children.length===1){row.querySelector("input").value="";return}
     row.remove();return;
   }
   const move=event.target.closest("[data-step-move]");
   if(!move)return;
   const sibling=Number(move.dataset.stepMove)<0?row.previousElementSibling:row.nextElementSibling;
   if(!sibling)return;
   if(Number(move.dataset.stepMove)<0)sibling.before(row);else sibling.after(row);
   row.querySelector("input").focus();
 });
 form.addEventListener("submit",async event=>{
   event.preventDefault();if(!manager())return;
   const id=field("id").value,days=[...form.querySelectorAll("[name=routine_day]:checked")].map(x=>Number(x.value));
   const steps=[...stepsBox.children].map(row=>({
     ...(row.dataset.stepId?{id:row.dataset.stepId}:{}),title:row.querySelector("input").value.trim()
   }));
   if(!days.length||!steps.length||steps.some(s=>!s.title)){
     status("Select a day and give every checklist step a name.");return;
   }
   const payload={title:field("title").value.trim(),notes:field("notes").value.trim(),
     assigned_to:field("assigned_to").value,time_of_day:field("time_of_day").value,
     days_of_week:days,steps};
   const submit=form.querySelector('[type="submit"]');submit.disabled=true;
   try{
     await api(id?"/api/routines/"+encodeURIComponent(id):"/api/routines",
       {method:id?"PUT":"POST",body:JSON.stringify(payload)});
     form.classList.add("hidden");await load();status(id?"Routine updated.":"Routine created.");
   }catch(err){status("Couldn't save routine: "+err.message)}
   finally{submit.disabled=false}
 });
 $("#routine-list").addEventListener("click",async event=>{
   const editButton=event.target.closest("[data-routine-edit]"),archive=event.target.closest("[data-routine-archive]");
   if(editButton){const item=state.routines.find(r=>r.id===editButton.dataset.routineEdit);if(item)edit(item);return}
   if(!archive)return;
   const item=state.routines.find(r=>r.id===archive.dataset.routineArchive);
   if(!item?.can_manage||!confirm('Archive "'+item.title+'"? Completion history will be preserved.'))return;
   try{
     await api("/api/routines/"+encodeURIComponent(item.id),{method:"DELETE"});
     await load();status("Routine archived, with history preserved.");
   }catch(err){status("Couldn't archive routine: "+err.message)}
 });
 $("#routine-list").addEventListener("change",async event=>{
   const box=event.target.closest("[data-routine-check]");if(!box)return;
   const routine=state.routines.find(r=>r.id===box.dataset.routineCheck);
   if(!routine?.due_today||!routine.can_complete){render();return}
   box.disabled=true;
   try{
     await api("/api/routines/"+encodeURIComponent(routine.id)+"/steps/"+
       encodeURIComponent(box.dataset.stepId)+"/status",
       {method:"PUT",body:JSON.stringify({date:activeDate(),completed:box.checked})});
     await load();
   }catch(err){status("Couldn't update checklist: "+err.message);await load()}
 });
 window.caldenRoutines={render,load,enter(page){
   if(page!=="routines")return;
   const routeDate=new URLSearchParams(location.search).get("date");
   if(routeDate&&/^\d{4}-\d{2}-\d{2}$/.test(routeDate))state.routineDate=routeDate;
   load();
 }};
})();
