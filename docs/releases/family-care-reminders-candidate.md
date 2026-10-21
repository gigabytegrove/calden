# Approved Family Care — Recurrence-Linked Notification Candidate

**Development candidate only.** This adds a missing piece of the already approved School, Maintenance and Pets modules. It does not create a second notification engine, require an external provider, or change the stable version.

## Implemented in source
- Existing two-minute maintenance cycle checks *today's* recurring care items using the same daily/weekly/monthly/yearly recurrence function already used by their UI, with the household timezone.
- Generates one durable inbox notification per **user, care item and scheduled occurrence date**, protected by a database unique index and idempotent insert. The existing notification trigger queues push delivery to previously enrolled devices.
- Completed/skipped occurrences, archived subjects, archived care items and disabled modules do not generate new reminders.
- School messages deliberately omit student names and school-record details. Only active authorized adult administrators/guardians or the authorized creator are recipients.
- Home upkeep/pet reminders go to permitted adult household administrators/planners, record owners or assigned adults. Children are excluded.
- The PostgreSQL `calden_can_view_care_notification` permission helper reevaluates current subject visibility, household enablement, record archive state, account status and active school guardian relationships. REST inbox, unread count, live WebSocket and queued push delivery use the same helper.
- Adds `care_due` to personal notification preferences and a real checkbox in the existing preferences UI. Already customized preferences retain their existing chosen kinds rather than being silently opted into new delivery categories.
- Adds Go recurrence tests and JavaScript source regression tests. The latter were executed using the JavaScript harness; **not** a real Node, PostgreSQL or Go runtime test.

## Explicit boundaries
- The original migration 037 implemented **due-day-only** reminders. The additive migration 040 now implements configurable advance notices as a development candidate; production and Android acceptance remain outstanding.
- A deactivated/cancelled notification may remain as an inaccessible historical row; queued delivery is blocked by current ACLs.
- This is not a guarantee of push arrival. OS restrictions and native Android acceptance remain to be tested.
- Migrations 037 and 038 must be run on a restored database and verified before production. Migration 038 preserves all existing event-confirmation, change-request and cancellation notification kinds even on a development database that applied the earlier 037 candidate. Web mobile behavior, accessibility and Go integration tests are outstanding.

## Verified RecipeCove contract, no invented integration
The RecipeCove repository currently implements `GET /api/v2/recipes`, `GET /api/v2/meal-plan`, `POST /api/v2/meal-plan` and `DELETE /api/v2/meal-plan/{id}`. The one-time administrator-issued API key, however, has scope **recipes:read** and is accepted only by the separately secured public recipe endpoint; the meal-plan endpoints require user session bearer authorization. Calden must **not** pretend that key permits two-way meal placement operations. A separately approved scoped API contract and idempotent sync protocol must exist on both services before any production two-way integration is represented as complete.


## Advance-reminder development candidate (migration 040)

The existing school, home upkeep and pet-care scheduler now supports a saved lead time
of 0, 1, 2, 3, 7, 14 or 30 calendar days before each occurrence. Zero is the
backwards-compatible default. The browser offers the setting in all three care
modules. A separate `care_advance` notification (one per person/item/target
occurrence) leaves the original `care_due` message and its uniqueness rule
untouched. Due dates use household-local calendar days; push and live delivery
reuse existing devices, inbox, settings and background maintenance.

Advanced school reminders deliberately exclude the student name, teacher,
room and title from notification payloads. Adult guardian authorization is
checked when creating reminders and again on read, stream and push delivery,
including guardian revocation and module-disable behavior. Recipient-specific
advance-notice delivery can be opted out separately in Personal Notifications.
Previously persisted notification preferences are **not** silently opted in.

Existing API clients may omit `reminder_days_before`; PUT keeps the existing
setting, while POST defaults to zero. Only the published choices are accepted.
The due-date maintenance job does not need an additional service. Advance notices
are generated on the calendar day matching the selected lead time while the
server runs; historical missed advances are not backfilled or converted into
repeat alerts. Android-native settings UI, restart/catch-up semantics, integration
tests and production acceptance still remain outstanding.

When an occurrence is completed or skipped, any outstanding matching care due/advance notifications are dismissed inside the same database transaction as the completion. Notification rows remain for historical accountability; delivery queries already exclude dismissed rows.
