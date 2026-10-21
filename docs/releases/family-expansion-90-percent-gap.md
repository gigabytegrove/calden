# Calden — approved family expansion gap audit (21 October 2026)

**Scope baseline:** [Approved Family Platform Expansion](../FAMILY-PLATFORM-EXPANSION.md), additive to [Product Direction](../PRODUCT-DIRECTION.md). The target is a full platform build ready for a **single user-operated acceptance round**, not a source-only milestone.

## Verified development commits this pass

- `a494523d` — school, home-maintenance and pet-care domain schemas, APIs and responsive web pages (migration 034).
- `bd3df949` — administrator-managed theme presets, custom names and safe icon catalog (migration 035).
- `d78880d4` — per-person delivery preferences and quiet-hour filtering in existing notification paths (migration 036).

Source-level JavaScript unit/contract assertions across eight family test files were executed in an isolated Node-compatible V8 harness: **28 passed using the corrected Rewards navigation selector in the tested source overlay. The corrected selector is included in the same follow-up GitHub commit as this audit.** All seven currently loaded browser scripts parse successfully. These are NOT a genuine `node --test` run with jsdom, a Go compilation, SQL integration suite or Android device run.

## Status against the eighteen approved capabilities

| Capability | Development state (not acceptance) | Remaining in code before 90% can be claimed |
| --- | --- | --- |
| Personal dashboards | Substantial web/backend foundation | Per-device layouts, persistence/migrations tests and Android parity |
| Child accounts/guardians | Server grants and web management in place | Guardian delegated actions, nuanced child data policies, Android client |
| Shared/private calendars | Existing calendar ACL foundation | Resource-level privacy, imported account ownership and negative integration tests |
| Reorderable widgets | Web customization present | Responsive saved positions, Android parity |
| RecipeCove two-way meals | **Blocked on actual scoped API contract** | RecipeCove 2.3.9 provides meal-plan routes for interactive sessions, but current API keys are limited to `recipes:read`. A revocable meal-plan write scope, revision/conflict handling and both sides of the sync must be built |
| Chores/rewards/allowances | Substantial web/Go development | Evidence/photo, guardian approval nuances, history and concurrency tests, Android |
| School activities | Student/term/schedule sources implemented | Calendar linkage, subscribed ICS intake, real reminders and privacy export |
| Shopping/supplies | Shared/private lists, categories, permitted assignees, manual stock/reorder thresholds, change history, and filterable web UI (migration 039) | Migration/ACL integration acceptance, Android parity, inventory adjustment workflows, RecipeCove ingredient preview |
| Home maintenance | Equipment, service schedule and history source | Attachments, reminders and detailed service workflows |
| Pets/vet care | Profiles, appointment/vaccine/medication schedule and history source | Permission granularity, reminders, care attachments |
| Routines | Ordered recurrence/completion foundation | Exceptions/streaks/guardian approval, real notification scheduling |
| Adult/child layouts | Basic web layouts and guardian setting | Full accessibility and native client equivalence |
| Custom module names/icons | Housewide server-validated module catalog and web UI | Mobile parity, accessibility acceptance |
| Household themes | Web preset tokens and personal appearance | Android token parity, contrast/accessibility acceptance |
| Per-person notifications | New persisted preferences and delivery filters | Advance notice overrides, delivery tests, Android recovery |
| Calendar import/sync | Legacy ICS/Google import baseline | Incremental subscription/provider synchronization, durable retry, conflicts |
| Device integrations | Existing Calden device registration; no optional household adapter complete | Consent-based supported adapter and explicit physical control authorization |
| Household exports/backup | Existing admin backup/recovery foundation | Versioned member-scoped exports, full documented restore validation |

## Work still needed before reaching 90%, excluding user's own acceptance tests

1. **Complete original notification reliability and shared Android features.** The current Calden Git tree contains **no Kotlin/Gradle project**. Its native Android source cannot be verified or expanded in this repository.
2. **RecipeCove two-way implementation** on both repositories with API-key scoping and safe conflict/idempotency state; never use a shared DB or scrape UI.
3. **Calendar subscription/sync engine**, including provider revisions, permission mapping, error visibility and bounded retries.
4. **Genuine recurrence-backed reminders** for school, home maintenance and pets using Calden's existing scheduler, not a second system.
5. **Selective privacy-safe exports and restored backup validation** and remaining household device adapter support.
6. **Finish partial features** (school/maintenance/pet care, shopping, routines, personalized layouts) and verify web/Android equivalent behavior.

## Execution environment

Direct GitHub DNS access is blocked from the local container, the available Go runtime is **1.23.2** while Calden requests **1.24**, and Gradle/Android sources are absent from this repository. GitHub connector can inspect/edit/commit source but cannot substitute compiled builds or database/real device acceptance.

**Completion judgment:** Several newly approved areas now have substantive, but incomplete server+web implementations. The full 18-capability expansion is **not 90% source-complete**, even after excluding end-user testing. No stable version bump or deployment is justified solely by these commits. Keep 1.4.8 as last declared stable version until full platform acceptance.

## 21 October 2026 follow-up — Shopping & Supplies

- `c2cf1e33`: extended Shopping & Supplies across Go APIs, migration 039, browser UI and unit/source tests with categories, permitted member assignments, low-stock summary counts, durable snapshots and private/shared history access.
- Preserves old clients' category/stock/assignment data when they omit the new keys; does not create a separate inventory service or require an external component.
- A dedicated PostgreSQL migration/trigger lifecycle test is included for future database-backed acceptance. The connected GitHub environment can inspect and commit source but cannot execute the required Go 1.24/PostgreSQL integration suite or Android tests here.
- This pass improves one approved feature. It does not independently establish 90% completion for the platform-wide expansion.
