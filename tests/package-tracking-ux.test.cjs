"use strict";
const test=require("node:test");
const assert=require("node:assert/strict");
const fs=require("node:fs");
const page=fs.readFileSync("web/index.html","utf8");
const ui=fs.readFileSync("web/packages.js","utf8");
const backend=fs.readFileSync("internal/api/packages.go","utf8");

test("package creation asks for name and tracking number, with optional overrides",()=>{
 const dialog=page.split('<dialog id="package-dialog"')[1]?.split("</dialog>")[0];
 assert.ok(dialog,"Packages dialog must exist");
 assert.match(dialog,/name="label"[^>]*required/);
 assert.match(dialog,/name="tracking_number"[^>]*required/);
 assert.match(dialog,/<details id="package-more-details"/);
 assert.match(dialog,/name="carrier"/);
 assert.match(dialog,/name="status"/);
 assert.match(dialog,/id="package-delete"/);
 assert.doesNotMatch(dialog,/id="package-archive"/);
});
test("package deletion is available from row and editor and is permanent on server",()=>{
 assert.match(ui,/data-package-delete/);
 assert.match(ui,/Permanently delete/);
 assert.match(ui,/method:"DELETE"/);
 assert.match(backend,/func \(s \*server\) deletePackage\(/);
 assert.match(backend,/DELETE FROM package_shipments/);
 assert.match(backend,/AND NOT archived/);
});
test("email onboarding is guided and technical piping is optional",()=>{
 assert.match(page,/id="package-email-provider"/);
 assert.match(page,/id="package-mailbox-form"/);
 assert.match(page,/myaccount\.google\.com\/apppasswords/);
 assert.match(page,/<details class="package-inbound-section" id="package-inbound-advanced"/);
 assert.match(page,/id="package-mailbox-advanced"/);
 assert.match(ui,/updateEmailProvider/);
});
test("Packages screen refreshes automatically while open",()=>{
 assert.match(ui,/window\.setInterval/);
 assert.match(ui,/state\.currentPage==="packages"/);
 assert.match(ui,/load\(true\)/);
});

test("the Packages page diagnoses an unavailable scraper instead of redirecting the user",()=>{
 assert.match(page,/id="packages-tracker-health"/);
 assert.match(ui,/\/api\/packages\/tracker-health/);
 assert.match(ui,/loadTrackerHealth/);
 const provider=fs.readFileSync("internal/api/package_provider.go","utf8");
 assert.match(provider,/claimManualTracking/);
 assert.match(provider,/last_tracking_check_at <= now\(\)-interval '2 minutes'/);
 assert.doesNotMatch(provider,/return publicErr/);
});

test("repair is an actionable in-app state, never a fake failed carrier request",()=>{
 const provider=fs.readFileSync("internal/api/package_provider.go","utf8");
 const runtime=fs.readFileSync("internal/api/package_runtime_repair.go","utf8");
 assert.match(provider,/parcelTrackingPending\(key\)/);
 assert.match(provider,/http\.StatusAccepted/);
 assert.match(provider,/"status":"repairing"/);
 assert.match(provider,/if parcelTrackingPending\(key\)\{return\}/);
 assert.match(runtime,/parcelTrackingEnabled\(ctx,db\)/);
 assert.match(runtime,/nextRetry=time\.Now\(\)\.Add\(10\*time\.Minute\)/);
 assert.match(ui,/trackerPreparing\(\)/);
 assert.match(ui,/Preparing tracker…/);
 assert.match(ui,/outcome\.status==="repairing"/);
 assert.match(ui,/loadTrackerHealth\(\)/);
});
