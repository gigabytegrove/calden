;(function caldenRewardsClient(){
 const host=$("#page-rewards");
 const status=t=>{$("#rewards-status").textContent=t||""};
 const managing=()=>["admin","planner"].includes(state.me?.role);
 const userName=id=>(state.users||[]).find(x=>x.id===id)?.display_name
   ||(state.rewards?.balances||[]).find(x=>x.user_id===id)?.name||"Family member";
 const centsToMoney=n=>new Intl.NumberFormat(undefined,{style:"currency",currency:"USD"}).format(Number(n||0)/100);
 const moneyToCents=value=>{
   const s=String(value||"").trim();
   if(!/^\d{1,7}(\.\d{1,2})?$/.test(s))return null;
   const [whole,frac=""]=s.split(".");
   const n=Number(whole)*100+Number(frac.padEnd(2,"0"));
   return Number.isSafeInteger(n)&&n>=1&&n<=100000000?n:null;
 };
 // randomUUID() may be unavailable on Calden's allowed plain-HTTP LAN origins.
 // getRandomValues() provides secure randomness there without requiring HTTPS.
 const newRequestID=()=>{
   if(window.crypto?.randomUUID)return window.crypto.randomUUID();
   if(!window.crypto?.getRandomValues)throw new Error("Your browser cannot securely identify this request");
   const b=new Uint8Array(16);window.crypto.getRandomValues(b);
   b[6]=(b[6]&15)|64;b[8]=(b[8]&63)|128;
   const h=[...b].map(v=>v.toString(16).padStart(2,"0")).join("");
   return h.slice(0,8)+"-"+h.slice(8,12)+"-"+h.slice(12,16)+"-"+h.slice(16,20)+"-"+h.slice(20);
 };
 const idempotency=form=>{
   if(!form.dataset.requestId)form.dataset.requestId=newRequestID();
   return form.dataset.requestId;
 };
 for(const id of ["reward-points-form","allowance-payment-form"]){
   $("#"+id).addEventListener("input",event=>{event.currentTarget.dataset.requestId=""});
 }
 const requestIds=new Map();
 const balances=()=>state.rewards?.balances||[];
 const catalog=()=>state.rewards?.catalog||[];
 function populatePeople(){
   if(!managing())return;
   const selected=state.users.filter(x=>x.active!==false);
   for(const selector of ["#reward-points-form [name=user_id]","#allowance-plan-form [name=user_id]","#allowance-payment-form [name=user_id]"]){
     const field=$(selector),current=field.value;
     field.innerHTML=selected.map(x=>`<option value="${escapeAttr(x.id)}">${escapeHTML(x.display_name)}</option>`).join("");
     if(selected.some(x=>x.id===current))field.value=current;
   }
 }
 function render(){
   if(!state.me)return;
   const enabled=state.settings?.rewards_enabled===true,manager=managing();
   $$(".rewards-nav").forEach(el=>el.classList.toggle("hidden",!enabled));
   if(!enabled)return;
   $$(".reward-manager").forEach(el=>el.classList.toggle("hidden",!manager));
   populatePeople();
   const people=balances();
   const user=people.find(x=>x.user_id===state.me.id);
   const relevant=manager?people:[user].filter(Boolean);
   $("#rewards-balances").innerHTML=relevant.length?relevant.map(person=>`
       <article class="panel rewards-balance-card">
         <p class="eyebrow">${escapeHTML(person.name)}</p>
         <div class="rewards-balance-numbers">
           <div><strong>${Number(person.points||0).toLocaleString()}</strong><span>Points</span></div>
           <div><strong>${centsToMoney(person.allowance_balance_cents)}</strong><span>Allowance due</span></div>
         </div>
       </article>`).join(""):'<div class="panel">No household balances available yet.</div>';
   const selfPoints=Number(user?.points||0);
   $("#rewards-catalog").innerHTML=catalog().length?catalog().map(reward=>`
     <article class="rewards-item" data-reward="${escapeAttr(reward.id)}">
       <div><strong>${escapeHTML(reward.title)}</strong><span class="muted">${escapeHTML(reward.description||"")}</span>
          <small>${reward.cost_points} points ${reward.active?"":"· Unavailable"}</small></div>
       <div class="rewards-controls">${reward.active?`<button type="button" class="button compact" data-claim-reward="${escapeAttr(reward.id)}"
            ${selfPoints<reward.cost_points?"disabled":""}>Request</button>`:""}
            ${manager?`<button type="button" class="button secondary compact" data-edit-reward="${escapeAttr(reward.id)}">Edit</button>`:""}</div>
     </article>`).join(""):'<p class="family-widget-empty">No rewards available yet.</p>';
   const claims=state.rewards?.claims||[];
   $("#rewards-claims").innerHTML=claims.length?claims.map(claim=>`
     <div class="rewards-item"><div><strong>${escapeHTML(claim.title)}</strong><small>${escapeHTML(userName(claim.user_id))} · ${claim.cost_points} points reserved · ${escapeHTML(claim.status)}</small></div>
       ${manager&&claim.status==="pending"?`<div class="rewards-controls">
         <button type="button" class="button compact" data-claim-action="${escapeAttr(claim.id)}" data-status="fulfilled">Fulfilled</button>
         <button type="button" class="button secondary compact" data-claim-action="${escapeAttr(claim.id)}" data-status="cancelled">Cancel / refund</button></div>`:""}
     </div>`).join(""):'<p class="family-widget-empty">No requests yet.</p>';
   const reviews=state.rewards?.pending_chores||[];
   $("#rewards-chore-approvals").innerHTML=reviews.length?reviews.map(c=>`
     <div class="rewards-item">
       <div><strong>${escapeHTML(c.title)}</strong><small>${escapeHTML(c.name)} · ${escapeHTML(c.date)} · ${c.points} points</small></div>
       <div class="rewards-controls"><button type="button" class="button compact" data-review-chore="${escapeAttr(c.chore_id)}" data-review-person="${escapeAttr(c.user_id)}" data-review-date="${escapeAttr(c.date)}" data-review-decision="approved">Approve</button>
       <button type="button" class="button secondary compact" data-review-chore="${escapeAttr(c.chore_id)}" data-review-person="${escapeAttr(c.user_id)}" data-review-date="${escapeAttr(c.date)}" data-review-decision="rejected">Reject</button></div>
     </div>`).join(""):'<p class="family-widget-empty">No completed chores waiting for a review.</p>';
   const plans=state.rewards?.plans||[];
   $("#allowance-plans-list").innerHTML=plans.length?plans.map(plan=>`
     <div class="rewards-item"><div><strong>${escapeHTML(userName(plan.user_id))}</strong>
       <small>${centsToMoney(plan.amount_cents)} · ${escapeHTML(plan.frequency)} · Next ${escapeHTML(plan.next_due_date)}
       ${plan.active?"":"· Paused"}</small></div>
       <button type="button" class="button secondary compact" data-edit-plan="${escapeAttr(plan.user_id)}">Edit</button>
     </div>`).join(""):'<p class="family-widget-empty">No repeating allowances configured.</p>';
   const activity=state.rewards?.activity||[];
   $("#rewards-activity").innerHTML=activity.length?activity.map(row=>`
     <div class="rewards-item">
       <div><strong>${escapeHTML(row.description)}</strong><small>${escapeHTML(userName(row.user_id))} · ${new Date(row.created_at).toLocaleDateString()}</small></div>
       <span class="rewards-amount">${row.type==="points"?(row.amount>0?"+":"")+Number(row.amount).toLocaleString()+" pts":
         (row.amount>0?"+":"−")+centsToMoney(Math.abs(row.amount))}</span>
     </div>`).join(""):'<p class="family-widget-empty">No point or allowance activity yet.</p>';
   window.caldenFamily?.renderDashboard();
 }
 async function load(){
   if(state.settings?.rewards_enabled!==true)return;
   try{
     const data=await api("/api/rewards/overview");
     state.rewards=data;
     render();status("");
   }catch(err){status("Couldn't load rewards: "+err.message)}
 }
 async function submit(form,path,method,payload,message){
   const button=form.querySelector('[type="submit"]');
   if(button)button.disabled=true;
   try{
     const result=await api(path,{method,body:JSON.stringify(payload)});
     await load();status(message);
     return result;
   }catch(err){status("Couldn't save: "+err.message);return null}
   finally{if(button)button.disabled=false}
 }
 $("#rewards-refresh").addEventListener("click",load);
 const catalogForm=$("#reward-catalog-form"),catField=n=>catalogForm.elements.namedItem(n);
 function clearCatalog(){
   catalogForm.reset();catField("id").value="";
 }
 $("#reward-catalog-clear").addEventListener("click",clearCatalog);
 catalogForm.addEventListener("submit",async event=>{
   event.preventDefault();if(!managing())return;
   const id=catField("id").value;
   const payload={title:catField("title").value.trim(),description:catField("description").value.trim(),
      cost_points:Number(catField("cost_points").value),active:catField("active").checked};
   const ok=await submit(catalogForm,id?"/api/rewards/catalog/"+encodeURIComponent(id):"/api/rewards/catalog",
     id?"PUT":"POST",payload,id?"Reward updated.":"Reward added.");
   if(ok)clearCatalog();
 });
 $("#rewards-catalog").addEventListener("click",async event=>{
   const edit=event.target.closest("[data-edit-reward]");
   if(edit){
     if(!managing())return;
     const reward=catalog().find(x=>x.id===edit.dataset.editReward);if(!reward)return;
     catField("id").value=reward.id;catField("title").value=reward.title;
     catField("description").value=reward.description;
     catField("cost_points").value=reward.cost_points;catField("active").checked=reward.active;
     catField("title").focus();return;
   }
   const claim=event.target.closest("[data-claim-reward]");
   if(!claim)return;
   const reward=catalog().find(x=>x.id===claim.dataset.claimReward);
   if(!reward?.active)return;
   if(!confirm('Request "'+reward.title+'" for '+reward.cost_points+' points?'))return;
   claim.disabled=true;
   let requestId;
   try { requestId=requestIds.get(reward.id)||newRequestID(); }
   catch(err){status(err.message);claim.disabled=false;return}
   requestIds.set(reward.id,requestId);
   try{
     await api("/api/rewards/catalog/"+encodeURIComponent(reward.id)+"/claim",
       {method:"POST",body:JSON.stringify({request_id:requestId})});
     requestIds.delete(reward.id);await load();status("Reward requested. Points are reserved until it is fulfilled or cancelled.");
   }catch(err){status("Couldn't request reward: "+err.message);claim.disabled=false}
 });
 $("#rewards-claims").addEventListener("click",async event=>{
   const btn=event.target.closest("[data-claim-action]");
   if(!btn||!managing())return;
   if(!confirm((btn.dataset.status==="fulfilled"?"Mark this reward fulfilled":"Cancel and refund points")+"?"))return;
   btn.disabled=true;
   try{
     await api("/api/rewards/claims/"+encodeURIComponent(btn.dataset.claimAction),
       {method:"PUT",body:JSON.stringify({status:btn.dataset.status})});
     await load();status("Reward request updated.");
   }catch(err){status("Couldn't update request: "+err.message);btn.disabled=false}
 });
 $("#rewards-chore-approvals").addEventListener("click",async event=>{
   const btn=event.target.closest("[data-review-chore]");
   if(!btn||!managing())return;
   const approve=btn.dataset.reviewDecision==="approved";
   if(!confirm(approve?"Approve this completed chore and award its points?":"Reject this completed chore for points?"))return;
   btn.disabled=true;
   try{
     await api("/api/rewards/chores/"+encodeURIComponent(btn.dataset.reviewChore)+"/review",
       {method:"POST",body:JSON.stringify({user_id:btn.dataset.reviewPerson,date:btn.dataset.reviewDate,
         decision:btn.dataset.reviewDecision,note:""})});
     await load();status(approve?"Chore approved and points awarded.":"Chore review recorded.");
   }catch(err){status("Couldn't review chore: "+err.message);btn.disabled=false}
 });
 $("#reward-points-form").addEventListener("submit",async event=>{
   event.preventDefault();if(!managing())return;
   const form=event.currentTarget,el=n=>form.elements.namedItem(n);
   let requestId;
   try{requestId=idempotency(form)}catch(err){status(err.message);return}
   const payload={user_id:el("user_id").value,delta:Number(el("delta").value),
     reason:el("reason").value.trim(),request_id:requestId};
   const ok=await submit(form,"/api/rewards/points/adjust","POST",payload,"Points adjusted and recorded in history.");
   if(ok){form.dataset.requestId="";el("delta").value="";el("reason").value=""}
 });
 const planForm=$("#allowance-plan-form");
 $("#allowance-plans-list").addEventListener("click",event=>{
   const button=event.target.closest("[data-edit-plan]");if(!button||!managing())return;
   const p=(state.rewards?.plans||[]).find(x=>x.user_id===button.dataset.editPlan);if(!p)return;
   const el=n=>planForm.elements.namedItem(n);
   el("user_id").value=p.user_id;el("amount").value=(p.amount_cents/100).toFixed(2);
   el("frequency").value=p.frequency;el("first_due").value=p.next_due_date;
   el("active").checked=p.active;el("amount").focus();
 });
 planForm.elements.namedItem("first_due").value=dateInputValue(new Date());
 planForm.addEventListener("submit",async event=>{
   event.preventDefault();if(!managing())return;
   const el=n=>planForm.elements.namedItem(n),amount=moneyToCents(el("amount").value);
   if(amount===null){status("Enter a valid allowance amount with no more than two decimal places.");return}
   await submit(planForm,"/api/rewards/allowances/plan","PUT",{
     user_id:el("user_id").value,amount_cents:amount,frequency:el("frequency").value,
     first_due:el("first_due").value,active:el("active").checked
   },"Recurring allowance schedule saved.");
 });
 $("#allowance-payment-form").addEventListener("submit",async event=>{
   event.preventDefault();if(!managing())return;
   const form=event.currentTarget,el=n=>form.elements.namedItem(n),
      amount=moneyToCents(el("amount").value);
   if(amount===null){status("Enter a valid paid amount with no more than two decimal places.");return}
   if(!confirm("Record "+centsToMoney(amount)+" as already paid? Calden will not transfer funds."))return;
   let requestId;
   try{requestId=idempotency(form)}catch(err){status(err.message);return}
   const payload={user_id:el("user_id").value,amount_cents:amount,note:el("note").value.trim(),
     request_id:requestId};
   const ok=await submit(form,"/api/rewards/allowances/payment","POST",payload,"Allowance payment recorded.");
   if(ok){form.dataset.requestId="";el("note").value=""}
 });
 window.caldenRewards={render,load,enter(page){if(page==="rewards")load()}};
})();
