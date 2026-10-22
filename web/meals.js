;(function CaldenRecipeCoveMeals(){
 const root=document.getElementById("page-meals");
 if(!root)return;
 const form=document.getElementById("meals-form"),field=name=>form.elements.namedItem(name);
 const message=document.getElementById("meals-status"),schedule=document.getElementById("meals-schedule");
 const start=document.getElementById("meals-start"),refresh=document.getElementById("meals-refresh");
 const save=document.getElementById("meals-save"),clear=document.getElementById("meals-clear");
 const previous=document.getElementById("meals-previous"),next=document.getElementById("meals-next");
 const pageTitle=document.getElementById("meals-range-title");
 let meals=[],recipes=[],initialized=false,revision=0,busy=false,ready=false;
 const dayString=date=>[date.getFullYear(),String(date.getMonth()+1).padStart(2,"0"),String(date.getDate()).padStart(2,"0")].join("-");
 const isoToday=()=>dayString(new Date());
 const plusDays=(day,n)=>{
  const d=new Date(day+"T12:00:00");
  if(Number.isNaN(d.getTime()))throw Error("Choose a valid starting date.");
  d.setDate(d.getDate()+n);return dayString(d);
 };
 const adult=()=>!state.me?.is_child&&state.me?.role!=="restricted";
 const safeNumber=value=>Number.isFinite(Number(value))?Number(value):0;
 function status(text){message.textContent=text||""}
 function setBusy(active){
  busy=active;
  for(const control of [refresh,previous,next,start,save,clear])control.disabled=active;
  for(const control of schedule.querySelectorAll("button"))control.disabled=active;
  form.setAttribute("aria-busy",String(active));
 }
 function reset(){
  if(busy)return;
  form.reset();field("id").value="";field("day").value=isoToday();
  field("meal").value="dinner";field("servings").value="4";
  form.dataset.requestId="";
  document.getElementById("meals-form-title").textContent="Plan a meal";
  save.textContent="Save meal";
 }
 function newID(){
  if(window.crypto?.randomUUID)return window.crypto.randomUUID();
  if(!window.crypto?.getRandomValues)throw Error("Secure random IDs are unavailable in this browser");
  const bytes=new Uint8Array(16);window.crypto.getRandomValues(bytes);
  return [...bytes].map(v=>v.toString(16).padStart(2,"0")).join("");
 }
 function clearPlan(){
  meals=[];recipes=[];ready=false;schedule.replaceChildren();
  field("recipe_id").replaceChildren(new Option("Select a RecipeCove recipe",""));
  pageTitle.textContent="";
 }
 function displayDay(day){
  const date=new Date(day+"T12:00:00");
  return Number.isNaN(date.getTime())?day:date.toLocaleDateString(undefined,{weekday:"long",month:"short",day:"numeric",year:"numeric"});
 }
 function buildButton(label,action,id){
  const button=document.createElement("button");
  button.type="button";button.className="button secondary compact";
  button.textContent=label;button.dataset[action]=String(id);button.disabled=busy;
  return button;
 }
 function buildEntry(meal){
  const entry=document.createElement("article");entry.className="meals-entry";
  const info=document.createElement("div"),name=document.createElement("strong");
  name.textContent=meal.recipe_title||"Recipe";
  const detail=document.createElement("small");
  detail.textContent=[meal.meal,safeNumber(meal.servings)+" servings"].join(" · ");
  info.append(name,detail);
  if(meal.note){const note=document.createElement("span");note.className="muted";note.textContent=meal.note;info.append(note)}
  const actions=document.createElement("div");actions.className="meals-actions";
  actions.append(buildButton("Edit","mealEdit",meal.id),buildButton("Remove","mealDelete",meal.id));
  entry.append(info,actions);return entry;
 }
 function render(){
  schedule.replaceChildren();
  const from=start.value,to=plusDays(from,13);
  pageTitle.textContent=displayDay(from)+" – "+displayDay(to);
  if(!meals.length){
   const empty=document.createElement("p");empty.className="muted";
   empty.textContent="No meals planned for these two weeks.";
   schedule.append(empty);return;
  }
  const groups=new Map();
  for(const meal of meals){
   if(!meal||typeof meal.day!=="string"||meal.day<from||meal.day>to)continue;
   if(!groups.has(meal.day))groups.set(meal.day,[]);
   groups.get(meal.day).push(meal);
  }
  for(const day of [...groups.keys()].sort()){
   const group=document.createElement("section");group.className="meals-day";
   const heading=document.createElement("h4");heading.className="meals-day-heading";heading.textContent=displayDay(day);
   group.append(heading);
   for(const meal of groups.get(day).sort((a,b)=>["breakfast","lunch","dinner","snack","anytime"].indexOf(a.meal)-["breakfast","lunch","dinner","snack","anytime"].indexOf(b.meal)))
    group.append(buildEntry(meal));
   schedule.append(group);
  }
  if(!schedule.children.length){
   const empty=document.createElement("p");empty.className="muted";
   empty.textContent="No meals planned for these two weeks.";schedule.append(empty);
  }
 }
 async function load(){
  if(!adult()){revision++;clearPlan();return}
  if(busy)return;
  if(!start.value)start.value=isoToday();
  const run=++revision;
  status("Loading your RecipeCove meal plan…");
  refresh.disabled=true;
  schedule.setAttribute("aria-busy","true");
  try{
   const cfg=await api("/api/integrations/recipecove/status");
   if(run!==revision)return;
   if(!cfg.configured){
    clearPlan();status("RecipeCove is not connected. An administrator can configure it in the server settings.");return;
   }
   const from=start.value,to=plusDays(from,13);
   const [recipeData,mealData]=await Promise.all([
    api("/api/integrations/recipecove/recipes"),
    api("/api/integrations/recipecove/meals?start="+encodeURIComponent(from)+"&end="+encodeURIComponent(to))
   ]);
   if(run!==revision)return;
   if(!Array.isArray(recipeData.items)||!Array.isArray(mealData.items))
    throw Error("RecipeCove returned an unexpected meal-plan format.");
   recipes=recipeData.items;meals=mealData.items;
   const selected=field("recipe_id").value;
   field("recipe_id").replaceChildren(new Option("Select a RecipeCove recipe",""));
   for(const recipe of recipes){
    if(!Number.isSafeInteger(Number(recipe.id))||Number(recipe.id)<1)continue;
    field("recipe_id").add(new Option(String(recipe.title||"Untitled recipe"),String(recipe.id)));
   }
   if(selected)field("recipe_id").value=selected;
   render();ready=true;initialized=true;status("");
  }catch(err){
   if(run!==revision)return;
   clearPlan();status("Could not load RecipeCove: "+err.message);
  }finally{
   if(run===revision){refresh.disabled=false;schedule.setAttribute("aria-busy","false")}
  }
 }
 async function performWrite(action,success){
  if(busy)return;
  ++revision;setBusy(true);status("Saving your changes in RecipeCove…");
  try{
   await action();
   setBusy(false);reset();await load();status(success);
  }catch(err){status("Could not save changes: "+err.message)}
  finally{setBusy(false)}
 }
 form.addEventListener("submit",async event=>{
  event.preventDefault();if(!adult()||busy||!ready)return;
  const selected=field("id").value;
  const payload={day:field("day").value,meal:field("meal").value,
   recipe_id:Number(field("recipe_id").value),servings:Number(field("servings").value),
   note:field("note").value.trim()};
  if(!payload.recipe_id||!payload.day||!Number.isFinite(payload.servings)||payload.servings<=0||payload.servings>100){
   status("Choose a recipe, day and valid serving count.");return;
  }
  if(!selected){
   try{form.dataset.requestId=form.dataset.requestId||newID()}
   catch(err){status(err.message);return}
   payload.request_id=form.dataset.requestId;
  }
  await performWrite(()=>api("/api/integrations/recipecove/meals"+(selected?"/"+encodeURIComponent(selected):""),{
   method:selected?"PUT":"POST",body:JSON.stringify(payload)
  }),selected?"Meal updated in RecipeCove.":"Meal added in RecipeCove.");
 });
 schedule.addEventListener("click",async event=>{
  if(busy||!ready||!adult())return;
  const target=event.target.closest("[data-meal-edit],[data-meal-delete]");
  if(!target||!schedule.contains(target))return;
  const id=Number(target.dataset.mealEdit||target.dataset.mealDelete);
  const item=meals.find(m=>Number(m.id)===id);
  if(!item)return;
  if(target.dataset.mealEdit){
   field("id").value=String(item.id);field("day").value=item.day;
   field("meal").value=item.meal;field("recipe_id").value=String(item.recipe_id);
   field("servings").value=String(item.servings);field("note").value=item.note||"";
   document.getElementById("meals-form-title").textContent="Edit planned meal";
   save.textContent="Update meal";
   form.scrollIntoView({block:"nearest",behavior:"smooth"});return;
  }
  if(!confirm("Remove this planned meal from RecipeCove?"))return;
  await performWrite(()=>api("/api/integrations/recipecove/meals/"+encodeURIComponent(id),{
   method:"DELETE"
  }),"Meal removed from RecipeCove.");
 });
 refresh.addEventListener("click",load);
 clear.addEventListener("click",reset);
 start.addEventListener("change",()=>{revision++;ready=false;load()});
 previous.addEventListener("click",()=>{start.value=plusDays(start.value||isoToday(),-14);revision++;ready=false;load()});
 next.addEventListener("click",()=>{start.value=plusDays(start.value||isoToday(),14);revision++;ready=false;load()});
 reset();
 window.caldenMeals={enter(page){
  if(page!=="meals"||state.settings?.meals_enabled===false){revision++;clearPlan();return}
  if(!initialized)reset();
  load();
 }};
})();