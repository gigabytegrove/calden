# Calden family care — school, home upkeep, and pets (development candidate)

**Status: unreleased source implementation.** This change is additive and stays within the approved family expansion. No version bump, no production deploy, no claims of real reminders or Android parity. Existing calendar/events, user identities, data, permissions, backup architecture and update processes are unchanged.

## What this candidate implements

- Three addressable, responsive web sections: **/school**, **/maintenance**, and **/pets**.
- Optional household-admin feature switches for all three modules. The initial state is OFF and disabling never drops records.
- School: student records with optional linkage to an existing child account, school name, grade, academic terms, school activities, deadlines and closure records, place/teacher/contact notes and responsibility.
- Home upkeep: household equipment/asset records, inspection and servicing schedules, responsible person, vendor/contact, notes and archived history.
- Pets: pet profile, appointment, vaccination and medication schedules, assigned person, veterinarian/place information and completion history.
- Shared daily, weekly, monthly and yearly recurrence calculation, including month-end and leap-year cases; one-off overdue items remain actionable.
- Saved completion/skip history by occurrence date and person; archived subjects/items preserve this history.
- Per-record access controls: child accounts cannot access these sections; linked school records are visible only to current guardians or household admins, and unlinked school records are private to their creator or household admins.
- Audit entries for subject, schedule, term and completion changes.
- Safe frontend rendering of names, notes, teachers and veterinary details; no HTML injection from user-provided content.

## Schema and API

Migration 034 creates `family_care_subjects`, `family_school_terms`, `family_care_items`, `family_care_occurrences` with foreign keys and uniqueness constraints. Each optional feature has its own enable setting.

Authenticated endpoints:
- `GET/POST /api/family/care/{kind}`
- `PUT /api/family/care/{kind}/{id}`
- `GET/POST /api/family/care/{kind}/{id}/items`
- `PUT /api/family/care/{kind}/{id}/items/{item_id}`
- `PUT /api/family/care/{kind}/{id}/items/{item_id}/occurrences/{date}`
- `GET /api/family/care/{kind}/{id}/history`
- `GET/POST /api/family/care/school/{id}/terms`

Supported `kind`: `school`, `maintenance`, `pets`. Item kinds are validated per section. The family member or planner who owns the record may modify it, and authorized household admins can manage it. An unrelated adult cannot access linked school information.

## Honest completion boundaries

This is NOT the entire approved school, equipment and pet feature scope. Still needed before acceptance:
- Creating or linking school appointments into existing Calden calendars, school ICS source mapping/subscription and duplicate detection.
- Automated and configurable notifications using the existing notification/reminder pipeline (no competing scheduler).
- Attachment storage and safe viewing/export, equipment service vendor document records, and more advanced care schedules.
- Additional student privacy tests, calendar cross-link permissions, edge-case recurrence confirmation.
- Full server integration tests against PostgreSQL migrations; web accessibility/end-to-end tests; Android UI parity.
- Independent archival restore on an isolated database and field testing before production deployment.

The source-level checks executed here cover JavaScript syntax, route-to-handler contracts, school access clauses and non-destructive schema. The new Go unit tests are committed but NOT yet executable in this environment's older Go toolchain.

**Do not label this candidate 90%-complete by itself.**
