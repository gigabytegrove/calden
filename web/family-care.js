;(function caldenCare(){
 const kinds={
  school:{title:"School",types:[["event","School activity"],["deadline","Deadline"],["closure","Closure"]]},
  maintenance:{title:"Home upkeep",types:[["service","Service"],["check","Inspection / check"]]},
  pets:{title:"Pet care",types:[["appointment","Appointment"],["vaccination","Vaccination"],["medication","Medication"]]}
 };
 const models={};
 const esc=x=>escapeHTML(String(x??""));
 const attr=x=>escapeAttr(String(x??""));
 function today(){return dateInputValue(new Date())}
 function cache(kind){
  if(!models[kind])models[kind]={subjects:[],subjectID:null,items:[],history:[],terms:[],children:[]};
  return models[kind];
 }
 const root=kind=>document.getElementById("page-"+kind);
 const el=(kind,key)=>root(kind).querySelector("[data-care-"+key+"]");
 const base=kind=>"/api/family/care/"+kind;
 const chosen=kind=>cache(kind).subjects.find(s=>s.id===cache(kind).subjectID);
 function status(kind,message){el(kind,"status").textContent=message||""}
 function isEnabled(kind){return state.settings?.[kind+"_enabled"]===true}
 function renderNav(){
  for(const kind of Object.keys(kinds)){
   document.querySelectorAll(".care-"+kind+"-nav").forEach(x=>x.classList.toggle("hidden",!isEnabled(kind)||!!state.me?.is_child));
  }
 }
 function formValue(form,name){return form.elements.namedItem(name)}
 function dismissForms(kind){
  el(kind,"subject-form").classList.add("hidden");
  el(kind,"item-form").classList.add("hidden");
 }
 function subjectForm(kind,subject=null){
  const form=el(kind,"subject-form");
  form.reset();
  for(const [key,value] of Object.entries({
   id:subject?.id||"",name:subject?.name||"",details:subject?.details||"",
   school_name:subject?.school_name||"",grade:subject?.grade||"",
   child_user_id:subject?.child_user_id||""
  })){
   const control=formValue(form,key);if(control)control.value=value;
  }
  formValue(form,"archived").checked=!!subject?.archived;
  const linked=formValue(form,"child_user_id");
  if(linked){linked.disabled=!!subject;linked.title=subject?"Student links are preserved when a record is edited":""}
  el(kind,"subject-form-title").textContent=subject?"Edit "+subject.name:"Add "+(kind==="pets"?"a pet":kind==="school"?"a student":"equipment");
  form.classList.remove("hidden");
  el(kind,"item-form").classList.add("hidden");
  formValue(form,"name").focus();
 }
 function fillPeople(kind){
  const form=el(kind,"item-form"),assigned=formValue(form,"assigned_to");
  const previous=assigned.value;
  assigned.innerHTML='<option value="">Not assigned</option>'+
    (state.users||[]).filter(u=>u.active!==false).map(u=>'<option value="'+attr(u.id)+'">'+esc(u.display_name)+'</option>').join("");
  assigned.value=previous;
 }
 function fillStudents(kind){
  const form=el(kind,"subject-form"),node=formValue(form,"child_user_id");
  if(!node)return;
  const previously=node.value;
  node.innerHTML='<option value="">No linked account</option>'+
    cache(kind).children.map(c=>'<option value="'+attr(c.id)+'">'+esc(c.display_name)+'</option>').join("");
  node.value=previously;
 }
 function fillTerms(kind){
  const form=el(kind,"item-form"),node=formValue(form,"term_id");
  if(!node)return;
  const previous=node.value;
  node.innerHTML='<option value="">Not set</option>'+
    cache(kind).terms.map(term=>'<option value="'+attr(term.id)+'">'+esc(term.title)+" ("+esc(term.begins_on)+" – "+esc(term.ends_on)+')</option>').join("");
  node.value=previous;
 }
 function itemForm(kind,item=null){
  const subject=chosen(kind);
  if(!subject?.can_manage||subject.archived)return;
  dismissForms(kind);
  const form=el(kind,"item-form");
  form.reset();
  formValue(form,"kind").innerHTML=kinds[kind].types.map(([id,label])=>
   '<option value="'+attr(id)+'">'+esc(label)+'</option>').join("");
  fillPeople(kind);
  fillTerms(kind);
  for(const [key,value] of Object.entries({
   id:item?.id||"",kind:item?.kind||kinds[kind].types[0][0],
   title:item?.title||"",details:item?.details||"",
   place:item?.place||"",contact:item?.contact||"",
   starts_on:item?.starts_on||today(),repeat_rule:item?.repeat_rule||"none",
   term_id:item?.term_id||"",assigned_to:item?.assigned_to||""
  })){
   const control=formValue(form,key);if(control)control.value=value;
  }
  formValue(form,"archived").checked=!!item?.archived;
  el(kind,"item-form-title").textContent=item?"Edit scheduled item":"Schedule an item";
  form.classList.remove("hidden");formValue(form,"title").focus();
 }
 function render(kind){
  const m=cache(kind),subjects=el(kind,"subjects");
  renderNav();
  subjects.innerHTML=m.subjects.length?m.subjects.map(s=>
   '<button type="button" class="family-list-choice '+(s.id===m.subjectID?"active":"")+
   '" data-care-select="'+attr(s.id)+'"><strong>'+esc(s.name)+'</strong><small>'+
   (s.archived?"Archived · ":"")+(kind==="school"?esc(s.school_name||"Family school record"):"")+ '</small></button>'
  ).join(""):'<p class="family-widget-empty">No records yet. Add one to get started.</p>';
  const s=chosen(kind);
  el(kind,"subject-name").textContent=s?.name||"Choose or add a record";
  el(kind,"subject-info").textContent=s?
   (s.archived?"Archived · ":"")+(kind==="school"?[s.school_name,s.grade].filter(Boolean).join(" · "):s.details||""):"Select a record to manage its schedule.";
  el(kind,"edit-subject").classList.toggle("hidden",!s?.can_manage);
  el(kind,"new-item").classList.toggle("hidden",!s?.can_manage||!!s?.archived);
  el(kind,"new-subject").classList.toggle("hidden",!!state.me?.is_child);
  if(kind==="school"){
   el(kind,"term-form").classList.toggle("hidden",!s?.can_manage||!!s?.archived);
   el(kind,"terms").innerHTML=m.terms.length?m.terms.map(t=>
    '<div class="rewards-item"><strong>'+esc(t.title)+'</strong><small>'+esc(t.begins_on)+' – '+esc(t.ends_on)+'</small></div>'
   ).join(""):'<p class="family-widget-empty">No academic terms saved.</p>';
  }
  el(kind,"items").innerHTML=s?(m.items.length?m.items.map(item=>{
   const badge=item.archived?"Archived":item.next_due?"Next / due "+item.next_due:"No future date";
   return '<article class="rewards-item family-care-item"><div><strong>'+esc(item.title)+'</strong>'+
    '<small>'+esc(item.kind.replaceAll("_"," "))+' · '+esc(badge)+' · '+esc(item.repeat_rule)+'</small>'+
    (item.details?'<span class="muted">'+esc(item.details)+'</span>':"")+
    (item.place?'<small>Place: '+esc(item.place)+'</small>':"")+
    (item.contact?'<small>Contact: '+esc(item.contact)+'</small>':"")+'</div><div class="rewards-controls">'+
    (s.can_manage?'<button class="button secondary compact" data-care-edit-item="'+attr(item.id)+'">Edit</button>':"")+
    (s.can_manage&&!item.archived&&item.next_due?'<button class="button compact" data-care-complete="'+attr(item.id)+'" data-date="'+attr(item.next_due)+'">Complete</button>'+
     '<button class="button secondary compact" data-care-skip="'+attr(item.id)+'" data-date="'+attr(item.next_due)+'">Skip</button>':"")+
    '</div></article>';
  }).join(""):'<p class="family-widget-empty">Nothing scheduled yet.</p>'):
   '<p class="family-widget-empty">Choose a record to view its schedule.</p>';
  el(kind,"history").innerHTML=s?(m.history.length?m.history.map(entry=>
   '<div class="rewards-item"><div><strong>'+esc(entry.title)+'</strong><small>'+esc(entry.date)+' · '+esc(entry.status)+
   ' · '+esc(entry.recorded_by_name)+'</small>'+(entry.note?'<span class="muted">'+esc(entry.note)+'</span>':"")+
   '</div></div>').join(""):'<p class="family-widget-empty">No service or completion history yet.</p>'):"";
 }
 async function detail(kind){
  const m=cache(kind),s=chosen(kind);
  if(!s){m.items=[];m.history=[];m.terms=[];render(kind);return}
  const prefix=base(kind)+"/"+encodeURIComponent(s.id);
  const [items,history,terms]=await Promise.all([
   api(prefix+"/items"),api(prefix+"/history"),
   kind==="school"?api(prefix+"/terms"):Promise.resolve({items:[]})
  ]);
  m.items=items.items||[];m.history=history.items||[];m.terms=terms.items||[];
  render(kind);
 }
 async function load(kind){
  if(!isEnabled(kind)||state.me?.is_child)return;
  try{
   const m=cache(kind),result=await api(base(kind));
   m.subjects=result.items||[];
   if(!m.subjects.some(x=>x.id===m.subjectID))m.subjectID=m.subjects.find(x=>!x.archived)?.id||m.subjects[0]?.id||null;
   if(kind==="school"){
    try{const children=await api("/api/family/children");m.children=children.items||[]}catch{m.children=[]}
    fillStudents(kind);
   }
   await detail(kind);status(kind,"");
  }catch(err){status(kind,"Couldn't load records: "+err.message)}
 }
 for(const kind of Object.keys(kinds)){
  const panel=root(kind),m=cache(kind);
  panel.addEventListener("click",async event=>{
   const button=event.target.closest("button");if(!button)return;
   if(button.hasAttribute("data-care-new-subject")){subjectForm(kind);return}
   if(button.hasAttribute("data-care-cancel-subject")){el(kind,"subject-form").classList.add("hidden");return}
   if(button.hasAttribute("data-care-cancel-item")){el(kind,"item-form").classList.add("hidden");return}
   if(button.hasAttribute("data-care-refresh")){await load(kind);return}
   if(button.dataset.careSelect){
    m.subjectID=button.dataset.careSelect;dismissForms(kind);render(kind);
    try{await detail(kind)}catch(err){status(kind,err.message)}
   }
   if(button.hasAttribute("data-care-edit-subject")){const subject=chosen(kind);if(subject?.can_manage)subjectForm(kind,subject)}
   if(button.hasAttribute("data-care-new-item"))itemForm(kind);
   if(button.dataset.careEditItem){
    const item=m.items.find(x=>x.id===button.dataset.careEditItem);if(item)itemForm(kind,item);
   }
   if(button.dataset.careComplete||button.dataset.careSkip){
    const itemID=button.dataset.careComplete||button.dataset.careSkip;
    const item=m.items.find(x=>x.id===itemID),subject=chosen(kind);
    if(!item||!subject?.can_manage)return;
    const selected=button.dataset.date,statusValue=button.dataset.careSkip?"skipped":"completed";
    if(!confirm((statusValue==="skipped"?"Skip ":"Complete ")+'"'+item.title+'" for '+selected+"?"))return;
    const note=prompt("Optional note for the saved history (maximum 2,000 characters):","")??"";
    if(note.length>2000){status(kind,"Note is too long.");return}
    button.disabled=true;
    try{
     await api(base(kind)+"/"+encodeURIComponent(subject.id)+"/items/"+encodeURIComponent(itemID)+
       "/occurrences/"+encodeURIComponent(selected),{method:"PUT",body:JSON.stringify({status:statusValue,note})});
     await detail(kind);status(kind,"Saved "+statusValue+" in history.");
    }catch(err){status(kind,"Couldn't record activity: "+err.message);button.disabled=false}
   }
  });
  el(kind,"subject-form").addEventListener("submit",async event=>{
   event.preventDefault();
   const f=event.currentTarget,field=n=>formValue(f,n),id=field("id").value;
   const payload={name:field("name").value.trim(),details:field("details").value.trim(),
    archived:field("archived").checked,
    school_name:field("school_name")?.value.trim()||"",grade:field("grade")?.value.trim()||"",
    child_user_id:field("child_user_id")?.value||null};
   const path=base(kind)+(id?"/"+encodeURIComponent(id):"");
   try{
    const result=await api(path,{method:id?"PUT":"POST",body:JSON.stringify(payload)});
    m.subjectID=result.id;f.classList.add("hidden");await load(kind);
    status(kind,id?"Record updated.":"Record added.");
   }catch(err){status(kind,"Couldn't save record: "+err.message)}
  });
  el(kind,"item-form").addEventListener("submit",async event=>{
   event.preventDefault();
   const subject=chosen(kind);if(!subject?.can_manage)return;
   const f=event.currentTarget,field=n=>formValue(f,n),id=field("id").value;
   const payload={kind:field("kind").value,title:field("title").value.trim(),
    details:field("details").value.trim(),place:field("place").value.trim(),
    contact:field("contact").value.trim(),starts_on:field("starts_on").value,
    repeat_rule:field("repeat_rule").value,
    assigned_to:field("assigned_to").value||null,term_id:field("term_id")?.value||null,
    archived:field("archived").checked};
   const path=base(kind)+"/"+encodeURIComponent(subject.id)+"/items"+(id?"/"+encodeURIComponent(id):"");
   try{
    await api(path,{method:id?"PUT":"POST",body:JSON.stringify(payload)});
    f.classList.add("hidden");await detail(kind);status(kind,"Schedule saved.");
   }catch(err){status(kind,"Couldn't save schedule: "+err.message)}
  });
  const term=el(kind,"term-form");
  if(term)term.addEventListener("submit",async event=>{
   event.preventDefault();const s=chosen(kind);if(!s?.can_manage)return;
   const f=event.currentTarget;
   const payload={title:formValue(f,"title").value.trim(),
    begins_on:formValue(f,"begins_on").value,ends_on:formValue(f,"ends_on").value};
   try{
    await api(base(kind)+"/"+encodeURIComponent(s.id)+"/terms",
     {method:"POST",body:JSON.stringify(payload)});
    f.reset();await detail(kind);status(kind,"Academic term saved.");
   }catch(err){status(kind,"Couldn't save academic term: "+err.message)}
  });
 }
 window.caldenCare={renderNav,load,enter(page){if(kinds[page])load(page)}};
})();
