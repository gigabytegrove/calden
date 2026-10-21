;(function CaldenRecipeCoveMeals(){
 const root=document.getElementById("page-meals");
 if(!root)return;
 const form=document.getElementById("meals-form"),field=name=>form.elements.namedItem(name);
 const message=document.getElementById("meals-status"), schedule=document.getElementById("meals-schedule");
 const start=document.getElementById("meals-start");
 let meals=[],recipes=[],initialized=false;
 const dayString=(date)=>[date.getFullYear(),String(date.getMonth()+1).padStart(2,"0"),String(date.getDate()).padStart(2,"0")].join("-");
 const isoToday=()=>dayString(new Date());
 const plusDays=(day,n)=>{const d=new Date(day+"T12:00:00");d.setDate(d.getDate()+n);return dayString(d)};
 function status(text){message.textContent=text||""}
 function reset(){
  form.reset();field("id").value="";field("day").value=isoToday();
  field("meal").value="dinner";field("servings").value="4";
  form.dataset.requestId="";document.getElementById("meals-form-title").textContent="Plan a meal";
  document.getElementById("meals-save").textContent="Save meal";
 }
 function newID(){
  if(window.crypto?.randomUUID)return window.crypto.randomUUID();
  if(!window.crypto?.getRandomValues)throw Error("Secure random IDs are unavailable in this browser");
  const b=new Uint8Array(16);window.crypto.getRandomValues(b);
  return [...b].map(v=>v.toString(16).padStart(2,"0")).join("");
 }
 function clearPlan(){
  meals=[];recipes=[];schedule.replaceChildren();
  field("recipe_id").innerHTML='<option value="">Select a RecipeCove recipe</option>';
 }
 function render(){
  if(!meals.length){schedule.innerHTML='<p class="muted">No meals planned in the next 14 days.</p>';return}
  schedule.innerHTML=meals.map(m=>`<article class="meals-entry">
      <div><strong>${escapeHTML(m.recipe_title||"Recipe")}</strong>
        <small>${escapeHTML(m.day)} · ${escapeHTML(m.meal)} · ${Number(m.servings)} servings</small>
        ${m.note?`<span class="muted">${escapeHTML(m.note)}</span>`:""}
      </div>
      <div class="meals-actions">
        <button type="button" class="button secondary compact" data-meal-edit="${Number(m.id)}">Edit</button>
        <button type="button" class="button secondary compact" data-meal-delete="${Number(m.id)}">Remove</button>
      </div></article>`).join("");
 }
 async function load(){
  if(state.me?.is_child||state.me?.role==="restricted")return;
  if(!start.value)start.value=isoToday();
  status("Loading your RecipeCove meal plan…");
  try{
   const cfg=await api("/api/integrations/recipecove/status");
   if(!cfg.configured){
    clearPlan();status("Connect RecipeCove using the RECIPECOVE_URL and RECIPECOVE_SYNC_KEY deployment settings.");return;
   }
   const from=start.value,to=plusDays(from,13);
   const [recipeData,mealData]=await Promise.all([
    api("/api/integrations/recipecove/recipes"),
    api("/api/integrations/recipecove/meals?start="+encodeURIComponent(from)+"&end="+encodeURIComponent(to))
   ]);
   recipes=recipeData.items||[];meals=mealData.items||[];
   const current=field("recipe_id").value;
   field("recipe_id").innerHTML='<option value="">Select a RecipeCove recipe</option>'+
    recipes.map(r=>`<option value="${Number(r.id)}">${escapeHTML(r.title)}</option>`).join("");
   if(current)field("recipe_id").value=current;
   render();status("");
   initialized=true;
  }catch(err){status("Could not load RecipeCove: "+err.message)}
 }
 form.addEventListener("submit",async event=>{
  event.preventDefault();
  if(state.me?.is_child||state.me?.role==="restricted")return;
  const selected=field("id").value;
  const payload={
   day:field("day").value,meal:field("meal").value,
   recipe_id:Number(field("recipe_id").value),
   servings:Number(field("servings").value),
   note:field("note").value.trim()
  };
  if(!payload.recipe_id||!payload.day||!Number.isFinite(payload.servings)||payload.servings<=0||payload.servings>100){
   status("Choose a recipe, day and valid serving count.");return;
  }
  if(!selected){
   try{form.dataset.requestId=form.dataset.requestId||newID()}
   catch(err){status(err.message);return}
   payload.request_id=form.dataset.requestId;
  }
  const button=document.getElementById("meals-save");button.disabled=true;
  try{
   await api("/api/integrations/recipecove/meals"+(selected?"/"+encodeURIComponent(selected):""),{
    method:selected?"PUT":"POST",body:JSON.stringify(payload)
   });
   reset();await load();status(selected?"Meal updated in RecipeCove.":"Meal added in RecipeCove.");
  }catch(err){status("Could not save meal: "+err.message)}
  finally{button.disabled=false}
 });
 schedule.addEventListener("click",async event=>{
  const edit=event.target.closest("[data-meal-edit]");
  const remove=event.target.closest("[data-meal-delete]");
  if(!edit&&!remove)return;
  const id=Number((edit||remove).dataset[edit?"mealEdit":"mealDelete"]);
  const item=meals.find(m=>Number(m.id)===id);
  if(!item)return;
  if(edit){
   field("id").value=String(item.id);field("day").value=item.day;
   field("meal").value=item.meal;field("recipe_id").value=String(item.recipe_id);
   field("servings").value=String(item.servings);field("note").value=item.note||"";
   document.getElementById("meals-form-title").textContent="Edit planned meal";
   document.getElementById("meals-save").textContent="Update meal";
   form.scrollIntoView({block:"nearest",behavior:"smooth"});return;
  }
  if(!confirm("Remove this planned meal from RecipeCove?"))return;
  try{
   await api("/api/integrations/recipecove/meals/"+id,{method:"DELETE"});
   await load();status("Meal removed from RecipeCove.");
  }catch(err){status("Could not remove meal: "+err.message)}
 });
 document.getElementById("meals-refresh").addEventListener("click",load);
 document.getElementById("meals-clear").addEventListener("click",reset);
 start.addEventListener("change",load);
 reset();
 window.caldenMeals={enter(page){
   if(page!=="meals")return;
   if(!initialized)reset();
   load();
 }};
})();