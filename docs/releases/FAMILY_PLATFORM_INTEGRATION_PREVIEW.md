# Calden — Family platform integrated development branch

Status: **implementation committed for integration testing, not an accepted stable release**.

## Included in this integrated change

- Additive PostgreSQL migration 030 for five household record domains, read/write scoping and per-occurrence completion history.
- Household feature registry: administrators can enable/disable shopping, school, maintenance, pets and routines. Disabled modules and navigation hide without deleting saved records.
- Actual authenticated CRUD endpoints and browser pages for each household module. Shared or personally private entries, due dates, repeating schedules, notes, lists/groups, completion and deletion. Recurring completion dates are recorded separately from the underlying schedule.
- Configurable personal dashboard, widget ordering, appearance and simplified layouts, backed by migration 029 and authenticated user settings endpoints.
- Household theme presets, customized module display labels and icon selections; authorization enforced on write endpoints.
- Browser startup regression fixture updated for the new script and APIs. Added Go validation tests and web feature-contract tests.
- Existing calendar, bill payment, notification, backup, updater and deployment files preserved without reinterpretation.

## Explicitly NOT claimed as finished

- No native Android UI parity or validated device-build acceptance yet.
- No real RecipeCove integration, two-way sync or verified RecipeCove APIs.
- No direct external calendar synchronization (existing ICS/Google migration unaffected).
- No rewards/allowance ledgers, guardian relationships, detailed student/pet profiles, household device adapter ecosystem, or per-person notification channels/quiet-hours UI.
- No new reminders sourced from household records yet; record dates and repeat state are available for follow-up integration with the established reminder scheduler.
- No validated full integration test suite, PostgreSQL migration dry-run on an existing installation, end-to-end browser run, or signed release.
- Dark theme overrides are a first increment; screen-by-screen contrast/accessibility review is still required.

## Release acceptance

Before tagging: compile server with Go 1.24; run `go test ./...`, `npm ci && npm run build:css && npm run test:web`, test blank and migrated PostgreSQL installations, verify negative private-access and role permissions, test each route in an actual browser and mobile viewport, verify backup/restore, and review production notifications and the Android app. Build a release artifact with Docker and test a Dokploy upgrade with rollback ready. Only then bump the version and publish release notes that match delivered functionality.

## Preservation

[Original Product Direction](../PRODUCT-DIRECTION.md) and [Family Platform Expansion](../FAMILY-PLATFORM-EXPANSION.md) remain in force. Feature approval never implies that incomplete integrations have shipped.
