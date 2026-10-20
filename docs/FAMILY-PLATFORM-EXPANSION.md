# Calden — Approved Family Platform Expansion (2026-10-20)

Status: **Approved product requirements; implementation and release acceptance are separate.**

This document adds to [PRODUCT-DIRECTION.md](PRODUCT-DIRECTION.md). It does not replace, relax, or supersede the existing commitments. Calendar-first design, permissions, bill payment semantics, notifications, deployment, existing data, user experience, and RecipeCove boundaries remain mandatory.

## Non-negotiable baseline

- Self-hosted family calendar and household organizer, usable by individuals, couples and families; clear everyday language.
- Preserve existing calendar colors as calendar identity and member avatars as assignee identity; admin-managed per-calendar view/edit rights, recurring events and occurrence edits, migrations, import/export and bill-payment lifecycle (allocated, paid, cleared, split/partial and actual payer).
- Preserve Go backend, PostgreSQL and automatic migrations, Tailwind CSS v4 web interface, native Android application, Docker/Dokploy deployment, existing update behavior and in-place upgrades.
- Finish existing durable notifications and Android reliability before optional modules; retain the persistent inbox, authenticated direct delivery, Android recovery, personal reminders, and optional Monita household reminders. No routine user-facing Firebase/protocol messages.
- Household admins manage available features and household-wide theme defaults; individuals manage their own dashboard, appearance and notification preferences within permissions. Disabled modules retain data.
- RecipeCove is optional and autonomous; do not introduce a competing recipe system, shared database, invented API, or required dependency.
- No shell/placeholder pages, inert switches, or promises of released functionality without verified end-to-end implementation.

## Approved features and acceptance criteria

### 1. Personal dashboards
Each household member has a private configurable home screen showing their permitted schedules, chores, bills, routines, reminders and widgets. A household view shows only information that viewer may access. Preferences persist and synchronize between clients; layout can differ for mobile and desktop.

### 2. Child accounts and permissions
An administrator or authorized guardian can create a restricted child account, configure module access, approve sensitive actions, manage associated guardians and choose a simplified UI. No child account can change household security, integrations, other people's private calendars, or payment/allowance ledger data without permissions. Authorization is enforced by the API, not just by navigation hiding. Do not expose children's private details in notifications or exports to unauthorized members.

### 3. Shared and private calendars
Maintain administrator-created calendars, color identity and explicit per-member read/edit access. Add personal/private and shared calendar experiences without weakening existing permissions. Redact private titles, descriptions, people, places and reminder payloads when not authorized. Imported/synced calendars have an explicit owner and access mapping.

### 4. Rearrangeable dashboard widgets
Offer a widget catalog filtered by enabled modules and user permissions. Support reorder, show/hide and configuration through keyboard/touch controls, not drag alone. Store per-person widget IDs, versioned settings and responsive layout positions; handle removed/disabled widgets safely while preserving preferences.

### 5. RecipeCove two-way meal planning
An authorized household member can connect/disconnect a RecipeCove instance using revocable, minimal-scope credentials. Calden owns meal-calendar placement, household visibility, scheduled date/time and assignment; RecipeCove owns recipes/ingredients/instructions. Define explicit supported operations for creating/updating/deleting meal-plan placements on each side, capability negotiation, durable sync queue, external identity mapping, idempotency keys, change origins, revisions/conflict handling, retries and reconciliation. Integrations must remain useful offline with clearly shown sync state and must never silently overwrite conflicting changes. Implement or verify the actual RecipeCove API contract before shipping. Do not assume existing endpoints.

### 6. Recurring chores, rewards and allowances
Provide one-off and recurring assigned chores, due dates, completion history, optional photo/note evidence, parental review, points and configurable reward catalog. Keep points, allowance entitlements and actual payments in separate ledgers with audit history. No direct banking/payments requirement. Recurrence must not overwrite completed instances; guard against duplicated rewards during retries.

### 7. School activities and important dates
Support student-linked school calendars, academic terms, events, deadlines, teacher/room notes as appropriate, closures, reminders and guardian visibility. Allow ICS/subscribed calendar intake with clear source attribution. Minimize child-identifiable exports and notifications.

### 8. Household shopping and supply lists
Multiple shared lists with item quantity, unit, categories, assignee, purchased state, editing history and optional inventory/reorder thresholds. Permission-filter all lists. RecipeCove ingredient import requires preview, explicit confirmation, duplicate matching, quantity/unit normalization and source attribution; never modify an existing list silently.

### 9. Home maintenance schedules
Track home assets/equipment, recurring maintenance tasks, dates, vendor notes, responsibility, service history, attachments and reminders. Completion advances recurrence without destroying prior service history.

### 10. Pet care and veterinary reminders
Pet profiles, care assignments, appointment and vaccination reminders, medications and recurring schedules. Veterinary/medical details are restricted by household permissions. Clearly distinguish user-entered schedules from professional advice.

### 11. Family routines and checklists
Build reusable morning/evening or custom routines with ordered steps, per-person assignment, schedule, completion history, optional streaks and guardian approval. Each scheduled instance has durable completion state; skipped steps and exceptions are represented explicitly.

### 12. Adult and child layouts
Provide role/age-appropriate preset experiences using the same secure service and module system. Child layouts use larger touch targets and reduced complexity; adults retain comprehensive management views. Respect access rules regardless of selected layout. Accessible overrides permitted.

### 13. Custom feature names and icons
Allow household administrators to set friendly display labels and icons for modules and approved navigation items. Internally use stable canonical IDs; never let renaming change API identities, permissions or data ownership. Validate labels and restrict icons to a supported safe catalog.

### 14. Household theme presets
Support household default preset, palette and appearance; each member can opt for allowed individual light/dark/system, density and dashboard settings. Tokens should work consistently on web and native Android, meet contrast and reduced-motion requirements, and preserve accessibility.

### 15. Per-person notification preferences
Allow individual choices by event type, delivery route, advance notice and quiet hours, subject to household safety/administrative constraints. Respect calendar permissions, device permission state, deduplication and notification history. Integrate with existing notification pipeline and Monita; no second delivery architecture.

### 16. Calendar import and synchronization
Preserve existing ICS import/export and Google migration behavior. Add subscription sync using stable provider IDs, incremental sync, refresh controls and bounded retry. Bidirectional calendar editing requires provider-specific authenticated support and explicitly selected calendars; never pretend a read-only ICS feed supports writes. Surface conflicts and permission errors; do not duplicate events.

### 17. Optional household device integrations
Support an opt-in adapter capability with user-selected device systems, least-privilege credentials, connection health, scoped household actions and audit trail. Avoid hard dependency on external hubs or services. Integrations must not silently gain control of physical devices when a feature is enabled; require separate authorization and clear setup.

### 18. Export and backup of household information
Authorized admins can produce complete versioned household exports including records, metadata, calendars, modules, permissions, attachments and settings as applicable. Members can export only data they may access. Provide documented tested full restore and selective human-readable exports; encrypt or protect sensitive archives and never include plaintext integration tokens by default. Verify checksums, format/schema version, repeatable restoration, data preservation and recovery on an isolated environment.

## Cross-cutting implementation design

1. **Permissions:** Household -> members/guardian relationships -> calendar/module/resource ACLs. All list, read, write, notification, search, import, export and integration endpoints enforce access server-side.
2. **Feature registry:** Stable module ID, lifecycle availability, household enablement, permissions, dependencies, client capability metadata, navigation/widgets and notification types. A disabled module hides relevant surfaces and stops its work without deleting data. Never expose unfinished features as working toggles.
3. **Scheduling:** One durable recurrence/occurrence and reminder infrastructure; domain records for chores, school, pets, routines and maintenance. Preserve time zone and daylight-saving semantics and occurrence-specific edits.
4. **Sync:** Provider adapters with external IDs, sync cursor/revisions, origin tracking, outbox/inbox idempotency, retry/backoff, conflict resolution and user-visible connection state. No shared database between Calden and RecipeCove.
5. **UI:** Responsive Tailwind CSS v4 web and native Kotlin Android using consistent design tokens and names, addressable routes, proper empty/loading/error states, and no generic shell placeholders.
6. **Persistence:** Additive schema migrations, foreign keys, household scoping, historical/audit tables where relevant, backup validation, no destructive disable toggles.
7. **Deployment:** Preserve existing self-hosted Go/PostgreSQL/Docker compatibility and managed Dokploy operation; no new always-on auxiliary containers required for feature modules. Update the existing architecture, not a rewrite.

## Build order and completion gates

- **Gate 0 — complete existing commitments:** server/Android notification reliability, recurring events, bills, permissions, regression and upgrade coverage; verify real device behavior.
- **Gate 1 — shared foundations:** household/user preference APIs, feature registry, child/guardian authorization, themes, dashboards, widgets, audit/events.
- **Gate 2 — household tasks:** chores, rewards/allowances, routines, school, shopping, equipment maintenance and pet care using shared recurrence/reminders.
- **Gate 3 — connected experience:** RecipeCove contract and two-way sync, calendar subscription and provider sync, optional device adapters.
- **Gate 4 — portability and acceptance:** full exports/restores, migration and reconciliation tests, documented recovery, web/Android parity and production acceptance.

Each deliverable requires database/API tests, authorization tests (including child/private negative cases), cross-client tests, migration/restore tests, accessible UI behavior and clear documented operational states. Release notes must distinguish implemented features from planned requirements. Do not mark any gate accepted until tests and actual deployment/device validation pass.
