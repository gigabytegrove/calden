# Calden family rewards and allowances — integrated development candidate

**Status:** Source implementation committed as an unreleased development candidate. **Not production accepted, not a stable release, not yet tested against a running PostgreSQL database or Android app.** Calden 1.4.8 stays the stable release while validation continues.

## Delivered source functionality

- Household administrators/planners can enable an optional Rewards & Allowances module without deleting any data.
- Household chores (not generic tasks) can be assigned a configurable points reward from 0–10,000. Existing chores default to zero; older clients can omit the field without resetting an existing reward.
- A person marks their assigned chore done through the existing Tasks & Chores experience. A household administrator/planner reviews a completed instance before points are awarded.
- Chore approvals are keyed by chore, person and occurrence date. Repeated approval requests cannot grant twice, and a credited instance cannot be reopened or deleted through ordinary chore operations. Rejected chores are returned to In progress; after the assignee completes them again, they reappear for a new review without creating duplicate points.
- Household administrators/planners can define or deactivate reward options and adjust points with a mandatory reason. An adjustment cannot create a negative points balance.
- Family members can request available rewards only when their balance is sufficient. Points are reserved on request. Administrators/planners mark claims fulfilled, or cancel and refund points.
- Client request IDs and unique source constraints provide duplicate-submission protection for reward claims, manual adjustments, and recorded allowance payments.
- Weekly and monthly allowance plans store amounts in integer cents and produce due ledger entries when Rewards is opened, with unique plan/date keys. Monthly schedules keep the configured day across shorter months, including February and leap years.
- Household administrators/planners record payments already made outside the application. Calden **never processes or transfers money**.
- Each person can see their own balances, claims and history; household administrators/planners can review all household accounts.
- A dedicated **/rewards** web page includes balances, catalog, requests, pending chore approvals, reward editing, points adjustments, recurring allowances, payment entries and activity.
- A personal Rewards dashboard widget is supported, and the existing Tasks & Chores editor exposes reward points for chores.

## Persistence and permissions

Migration **032_family_rewards_allowances.sql** introduces chore reward-point configuration, approval history, an append-only points ledger, reward catalog and claims, allowance plans and allowance ledger. Existing events, bills, chores and calendar access remain unchanged. Disabling this module preserves all records.

Review operations serialize on the chore row. Point debits, refunds and adjustments serialize on the beneficiary user row, and all ledger mutations use transactions. Claim fulfillment and cancellation are mutually exclusive; cancelled claims are refunded once. Historical allowance due amounts survive plan edits.

### APIs (all authenticated)

- `GET /api/rewards/overview` — balances, catalogs, claims, pending completed chores, allowance plans and activity, filtered by role and member.
- `POST /api/rewards/catalog`, `PUT /api/rewards/catalog/{id}` — manager only.
- `POST /api/rewards/catalog/{id}/claim` — authenticated person requesting their own reward.
- `PUT /api/rewards/claims/{id}` — manager decision: `fulfilled` or `cancelled`.
- `POST /api/rewards/chores/{id}/review` — manager decision for specified assignee and occurrence.
- `POST /api/rewards/points/adjust` — manager-only signed points adjustment with reason and request ID.
- `PUT /api/rewards/allowances/plan` — manager-only weekly/monthly plan with amount in cents, first due date, and active state.
- `POST /api/rewards/allowances/payment` — manager-only record of an external payment, amount in cents, with request ID.
- `PUT /api/settings/features` with `{"rewards_enabled":true}` — administrator-only household visibility switch.

## Explicit operational boundaries

- **Points credit requires a manager approval**; completing a chore does not itself pay or credit points.
- **Recurring allowance accrual is lazy**: due entries are inserted when the Rewards view is loaded, not by a guaranteed unattended timed job. Weekly/monthly plan recurrence works through bounded catch-up and idempotent records, but an offline or unused instance may post entries later when opened.
- **No banking integration, payment settlement or independent account balance verification.**
- The existing Android client is not yet updated to display the Rewards interface. These endpoints support future native Kotlin integration but **Android parity is not claimed**.
- No dedicated reward approval push notifications are added in this candidate; existing calendar and chore notifications should not be misrepresented as reward-specific alerts.
- Existing chore series deletion is blocked when review history exists, to avoid erasing credited chore provenance. A richer chore archiving UX can be added later.

## Acceptance and release gates

1. Run `go test ./...` using Go 1.24+ with all dependencies.
2. Run `npm install --no-audit --no-fund && npm run build:css && npm run test:web`.
3. Apply migrations 029–032 on an **isolated restored backup**, validate schema constraints, existing recurrence, private calendar permissions, shopping and routine regression.
4. Integration tests with admin, planner, member and restricted users; duplicate review, concurrent review, concurrent claim, negative-balance rejection, stale idempotency key, claim cancel/retry, allowance double accrual, month-end recurrence and archived history.
5. Browser tests for all rewards screens, reduced-motion, narrow screen, keyboard-only controls, plain-HTTP LAN request IDs, optional module switch and return from disabled state.
6. Android compatibility and real-device reminder regression, including continued support for older clients omitting `reward_points`.
7. Confirm backup/restore carries all ledger data, then version bump, release changelog, packaged artifact and deployment acceptance before any production tag.

The scope remains additive to [approved family platform expansion](../FAMILY-PLATFORM-EXPANSION.md).
