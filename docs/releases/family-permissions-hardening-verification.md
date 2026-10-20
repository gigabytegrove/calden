# Calden — family permission and regression hardening

**Scope:** Corrections only to the existing, approved Calden family platform work. No new module, external dependency, banking integration, separate app or release tag.

**Candidate status:** Source-level regression checks passed. **No compiled Go build, PostgreSQL migration execution, running-browser acceptance, Android acceptance or production deployment has been verified.**

## Bugs and regressions corrected

1. The optional Rewards navigation used a single-element DOM selector followed by `.forEach()`, raising a browser error when the navigation was updated. It now uses the same multi-element selector as Shopping and Routines.
2. Existing source tests expected old JavaScript implementation details and incorrectly classified safe selectors and new retry IDs as failures. Assertions were updated to validate the intended behavior.
3. Child access to the shared Tasks/Chores endpoint now recognizes **either** approved module grant, then applies an independent server-side per-kind check. Tasks-only access does not reveal chores, and chores-only access does not reveal tasks.
4. Child chore responses exclude other assignees' per-occurrence status and avatar metadata.
5. Child notification access is now a guardian-revocable grant. The initial child default continues to permit notifications, preserving earlier user-facing behavior.
6. The personal dashboard never includes disallowed Notification widgets or inserts them automatically when every previously saved widget has become unauthorized.
7. Calendar revocation is reflected in REST notification reads, the live WebSocket query and queued push deliveries. Bill Pay review notices are excluded for child accounts even when not linked to a surviving calendar.
8. Push delivery checks current user activation, current calendar view permissions, child feature grants and the device/notification ownership relationship before leasing messages.
9. Background Bill Pay notification generation skips child accounts, preventing child-targeted billing notices from being newly created.

## Tests actually executed in the assistant environment

Five checked-in JavaScript test files were executed against the GitHub connector's exact file contents, with pending revisions substituted by blob SHA. An isolated JavaScript runtime supplied Node-like `node:test`, `assert`, `fs`, `path` and script syntax evaluation to execute their assertions.

| Test suite | Assertions passed |
| --- | ---: |
| `tests/family-platform.test.cjs` | 3 / 3 |
| `tests/family-routines.test.cjs` | 3 / 3 |
| `tests/family-rewards.test.cjs` | 4 / 4 |
| `tests/family-children.test.cjs` | 3 / 3 |
| `tests/family-security-regressions.test.cjs` | 5 / 5 |
| **Total** | **18 / 18** |

These exercises validate source-level behavior contracts, route wiring, selected privacy-policy SQL clauses, and syntax of the browser scripts. They do **not** execute a genuine `node --test` process, a browser DOM, Go handlers, SQL queries or Android code.

## Remaining release gates (not executed)

- `go test ./...` against the full repository: assistant container has **Go 1.23.2**, while Calden's `go.mod` requires **Go 1.24**, and direct GitHub/Golang proxy DNS access is blocked.
- `npm run test:web` with the actual repository, `jsdom`, and full Node testing.
- `npm run build:css` and production asset packaging.
- PostgreSQL migration, concurrent DB transaction and role-permission integration tests.
- Real WebSocket and push dispatch tests, including revocation during active connections and queued notification delivery.
- Native Kotlin Android and cross-device sign-in, privacy, notification and layout acceptance.
- End-to-end install, backup/recovery, production smoke tests and a stable release decision.

**Release status:** Calden 1.4.8 is unchanged; do not claim a stable release or production readiness from these source checks.
