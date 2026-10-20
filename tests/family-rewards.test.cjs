const test=require("node:test");
const assert=require("node:assert/strict");
const fs=require("node:fs");
const path=require("node:path");
const vm=require("node:vm");
const root=path.resolve(__dirname,"..");
const source=p=>fs.readFileSync(path.join(root,p),"utf8");

test("rewards client is parsed and loaded as addressable Calden page",()=>{
 const html=source("web/index.html"),app=source("web/app.js"),rewards=source("web/rewards.js");
 assert.doesNotThrow(()=>new vm.Script(rewards,{filename:"web/rewards.js"}));
 assert.doesNotThrow(()=>new vm.Script(app,{filename:"web/app.js"}));
 for(const id of ["page-rewards","rewards-balances","rewards-catalog","rewards-chore-approvals",
   "reward-catalog-form","reward-points-form","allowance-plan-form",
   "allowance-payment-form","settings-rewards-enabled","rewards-activity"]){
   assert.ok(html.includes('id="'+id+'"'),"missing "+id);
 }
 assert.match(html,/data-page="rewards" href="\/rewards"/);
 assert.match(html,/src="\/rewards\.js\?/);
 assert.ok(html.includes('id="rewards-review-history"'));
 assert.match(rewards,/Prepaid allowance/);
 assert.match(app,/rewards_enabled===true/);
 assert.match(app,/window\.caldenRewards\?\.enter\(page\)/);
 assert.match(app,/const caldenPages=new Set\(\["dashboard","shopping","routines","rewards"/);
});

test("rewards APIs are all authenticated, using manager checks on privileged writes",()=>{
 const api=source("internal/api/api.go"),backend=source("internal/api/rewards.go");
 for(const route of [
  "GET /api/rewards/overview","POST /api/rewards/catalog","PUT /api/rewards/catalog/{id}",
  "POST /api/rewards/catalog/{id}/claim","PUT /api/rewards/claims/{id}",
  "POST /api/rewards/chores/{id}/review","POST /api/rewards/points/adjust",
  "PUT /api/rewards/allowances/plan","POST /api/rewards/allowances/payment"
 ]) assert.ok(api.includes('mux.Handle("'+route+'", s.auth('),"missing "+route);
 for(const fn of ["rewardCatalogCreate","rewardCatalogUpdate","choreReviewReward",
  "pointAdjustment","resolveRewardClaim","saveAllowancePlan","recordAllowancePayment"]){
   const start=backend.indexOf("func (s *server) "+fn+"(");
   assert.ok(start>=0,fn+" unavailable");
   assert.match(backend.slice(start,start+280),/requiredRewardManager\(w,r\)/,fn+" isn't manager-gated");
 }
 assert.match(backend,/lockRewardUser\(r,tx/);
 assert.match(backend,/if balance<int64\(cost\)/);
 assert.match(backend,/oldReward!=rewardID/);
 assert.match(backend,/JOIN users u ON u\.id=p\.user_id AND u\.active=true/);
 assert.match(backend,/rv\.decision='rejected'/);
 assert.match(backend,/"reviews":reviews/);
 assert.match(backend,/ON CONFLICT \(plan_id,due_date\) WHERE kind='due' DO NOTHING/);
});

test("awarded chores and point movements retain their history and cannot be recreated",()=>{
 const chores=source("internal/api/chores.go"),migration=source("internal/store/migrations/032_family_rewards_allowances.sql");
 assert.match(chores,/reward_points/);
 assert.match(chores,/FROM family_chore_reviews/);
 assert.match(chores,/chore was already rewarded/i);
 assert.match(migration,/UNIQUE\(chore_id,user_id,occurrence_date\)/);
 assert.match(migration,/CREATE UNIQUE INDEX family_point_unique_source/);
 assert.match(migration,/CREATE UNIQUE INDEX allowance_due_unique/);
 assert.match(migration,/CREATE TABLE family_allowance_plans/);
 assert.match(migration,/ON CONFLICT\(key\) DO NOTHING/);
});

test("reward requests have a secure LAN-friendly ID and HTML-safe rendering",()=>{
 const client=source("web/rewards.js");
 assert.match(client,/getRandomValues/);
 assert.match(client,/request_id:requestId/);
 assert.match(client,/idempotency\(form\)/);
 assert.match(client,/requestIds\.set\(reward\.id,requestId\)/);
 assert.match(client,/escapeHTML\(reward\.title\)/);
 assert.match(client,/escapeHTML\(row\.description\)/);
 assert.doesNotMatch(client,/Math\.random/);
});
