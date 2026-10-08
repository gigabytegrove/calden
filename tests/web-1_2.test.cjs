"use strict";
const test=require("node:test"),assert=require("node:assert/strict"),fs=require("node:fs"),path=require("node:path");
const read=p=>fs.readFileSync(path.join(__dirname,"..",p),"utf8");
const js=read("web/app.js"),html=read("web/index.html"),css=read("web/app.css");
test("compact bill calendar state",()=>{
  assert.ok(js.includes('icon:"⌛"'));
  assert.ok(js.includes('icon:"✓",label:"Cleared"'));
  assert.ok(!js.includes('icon:"✓✓"'));
  assert.ok(js.includes('billPaidAvatar(e)'));
  assert.ok(css.includes("background:#ffc107"));
});
test("agenda is upcoming and filterable",()=>{
  for(const id of ["agenda-kind","agenda-range","agenda-calendar"])assert.ok(html.includes('id="'+id+'"'),id);
  assert.ok(js.includes("const from=new Date(),until=addDays"));
  assert.ok(js.includes('eventEndDate(e)<from'));
});
test("alerts can be dismissed and stay dismissed",()=>{
  assert.ok(js.includes("data-dismiss-notification"));
  assert.ok(read("internal/api/api.go").includes('DELETE /api/notifications/{id}'));
  assert.ok(read("internal/api/notifications.go").includes("dismissed_at IS NULL"));
  assert.ok(read("internal/store/migrations/015_notification_dismissals_and_reminder_recipients.sql").includes("dismissed_at"));
});
test("personal reminders target users",()=>{
  assert.ok(js.includes("reminder-recipient"));
  assert.ok(js.includes("recipient_user_id"));
  assert.ok(read("internal/api/resources.go").includes('"recipient_user_id": recipient'));
});
