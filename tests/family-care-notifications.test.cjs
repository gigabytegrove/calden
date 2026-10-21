const test=require("node:test"),assert=require("node:assert/strict"),fs=require("node:fs"),path=require("node:path");
const root=path.resolve(__dirname,"..");const read=p=>fs.readFileSync(path.join(root,p),"utf8");
test("care reminder maintenance is integrated into existing worker",()=>{
 const m=read("internal/api/notification_maintenance.go"),c=read("internal/api/care_notifications.go");
 assert.match(m,/generateCareNotifications\(scoped,db\)/);
 assert.match(c,/careOccurrenceValid\(x.starts,day,x.repeat\)/);
 assert.match(c,/family_care_occurrences/);
 assert.match(c,/kind='care_due'/);
 assert.match(c,/family_child_guardians/);
 assert.match(c,/ON CONFLICT \(user_id,care_item_id,kind,occurrence_start\)/);
});
test("care notification ACL is rechecked at read and delivery time",()=>{
 const migration=read("internal/store/migrations/037_family_care_reminders.sql");
 assert.match(migration,/calden_can_view_care_notification/);
 assert.match(migration,/u.is_child=false/);
 assert.match(migration,/s.archived=false/);
 assert.match(migration,/family_child_guardians/);
 for(const file of ["internal/api/notifications.go","internal/api/notification_stream.go","internal/push/worker.go"]){
  assert.match(read(file),/calden_can_view_care_notification\(n.care_item_id,n.user_id\)/,file);
 }
});
test("care opt-in is connected to personal notification preferences",()=>{
 const prefs=read("internal/api/notification_preferences.go");
 const html=read("web/index.html"),migration=read("internal/store/migrations/037_family_care_reminders.sql");
 assert.match(prefs,/"care_due":true/);
 assert.match(prefs,/"care_due"/);
 assert.match(html,/name="kind" value="care_due"/);
 assert.match(migration,/notification_care_occurrence_once/);
 assert.match(migration,/enabled_kinds/);
 for(const kind of ["event_confirmation","event_change_requested","event_confirmation_request","event_schedule_changed","event_cancelled"]){
   assert.ok(migration.includes("'"+kind+"'"),"lost pre-existing kind "+kind);
   assert.ok(read("internal/store/migrations/038_notification_kind_compatibility.sql").includes("'"+kind+"'"),"forward fix missing "+kind);
 }
});

// Source-contract test complements—not replaces—the Go/Postgres integration runs.
test("advance care reminders keep due-day notices and private records separate",()=>{
 const m=read("internal/store/migrations/040_family_care_advance_reminders.sql");
 const engine=read("internal/api/care_notifications.go");
 const care=read("internal/api/family_care.go");
 const ui=read("web/family-care.js"),html=read("web/index.html");
 const prefs=read("internal/api/notification_preferences.go");
 assert.match(m,/notification_care_advance_occurrence_once/);
 assert.match(m,/kind='care_advance'/);
 assert.match(m,/care_due','care_advance'/);
 assert.match(engine,/careAdvanceOccurrence\(x.starts,day,x.repeat,x.lead\)/);
 assert.match(engine,/WHERE kind='care_advance'/);
 assert.match(engine,/kind='care_due'/);
 assert.match(engine,/family_child_guardians/);
 assert.match(care,/COALESCE\(\$13::integer,reminder_days_before\)/);
 assert.match(care,/COALESCE\(\$12::integer,0\)/);
 assert.match(care,/careReminderLeadAllowed/);
 for(const file of ["internal/api/notifications.go","internal/api/notification_stream.go","internal/push/worker.go"]){
   assert.match(read(file),/n.kind NOT IN \('care_due','care_advance'\)/,file);
 }
 assert.equal((html.match(/name="reminder_days_before"/g)||[]).length,3);
 assert.match(html,/value="care_advance"/);
 assert.match(ui,/reminder_days_before:Number/);
 assert.match(prefs,/"care_advance":true/);
});

test("completing a care occurrence dismisses matching outstanding reminders atomically",()=>{
 const code=read("internal/api/family_care.go");
 assert.match(code,/UPDATE notifications SET dismissed_at=now\(\)/);
 assert.match(code,/kind IN \('care_due','care_advance'\)/);
 assert.match(code,/occurrence_start=\(\$2::date::timestamp AT TIME ZONE 'UTC'\)/);
 const dismiss=code.indexOf("UPDATE notifications SET dismissed_at=now()");
 const commit=code.indexOf('if err=tx.Commit(r.Context())',dismiss);
 assert.ok(dismiss>=0&&commit>dismiss,"reminder dismissal must be committed with completion");
});

test("guarded Postgres integration test verifies school privacy, due and advance independently",()=>{
 const file=read("internal/api/care_advance_integration_test.go");
 assert.match(file,/CALDEN_TEST_DATABASE_URL/);
 assert.match(file,/Refusing non-test database/);
 assert.match(file,/generateCareNotifications\(ctx,db\)/);
 assert.match(file,/family_child_guardians/);
 assert.match(file,/care_due/);
 assert.match(file,/care_advance/);
 assert.match(file,/Duplicate/);
});

test("care notification inbox shows the target date and opens the private authorized record",()=>{
 const backend=read("internal/api/notifications.go"),ui=read("web/app.js"),care=read("web/family-care.js");
 assert.match(backend,/n.care_item_id,cs.kind,cs.id/);
 assert.match(backend,/LEFT JOIN family_care_subjects cs ON cs.id=ci.subject_id/);
 assert.match(backend,/"care_subject_id": careSubjectID/);
 assert.match(ui,/const careReminder=item.kind==="care_due"\|\|item.kind==="care_advance"/);
 assert.match(ui,/careReminder\|\|item.all_day\?calendarDate\(startValue\)/);
 assert.match(ui,/window.caldenCare\?\.open\(item.care_kind,item.care_subject_id\)/);
 assert.match(care,/open\(kind,subjectID\)/);
});
