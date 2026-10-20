# Calden family home and shopping — integration candidate

**Implementation status:** Committed development work, **not a released stable version**. Built on the existing 1.4.8 source; do not change production without a tested migration and rollback copy.

## Included in this integrated change

- Personal dashboard preference persistence (migration 029), accessible widget ordering, widget selection, simple/standard dashboard density, and new-home appearance controls.
- Shopping-list and shopping-item persistence (migration 030); multiple lists, household-shared or owner-private access, owner-controlled archive/deletion, item quantity/unit/notes/purchased state, and audit entries.
- Responsive **/dashboard** and **/shopping** web routes in the existing Calden shell.
- Household-admin Shopping feature switch; disabling does not delete data and rejects unauthorized API access.
- Reused login authorization, user roles, event/notification data, tasks/chores and calendar access. Restricted members have view-only access to shared shopping lists.

## Access and data behavior

A list is visible to its creator, or to authenticated household accounts when marked shared. Only its creator may modify its name, sharing, archive state or delete it. Editing and marking items requires a non-restricted account plus list access; an archived list rejects item modifications. Private lists do not automatically become visible to administrators. All API endpoints use authenticated routes. SQL takes UUID IDs as parameters, and client-supplied titles and notes are escaped during rendering.

The Shopping module switch affects the interface and shopping API but never drops records. Existing core calendar features continue to be available.

## API contract (authenticated)

- `GET /api/me/dashboard` and `PUT /api/me/dashboard`: `{"appearance":"system|light|dark","layout":"standard|simple","widgets":["calendar","notifications","shopping"]}` (other supported widget IDs: tasks, chores, bills, agenda).
- `GET/POST /api/shopping/lists`.
- `PUT/DELETE /api/shopping/lists/{id}`.
- `GET/POST /api/shopping/lists/{id}/items`.
- `PUT/DELETE /api/shopping/lists/{id}/items/{item_id}`.
- `PUT /api/settings/features` with `{"shopping_enabled":true|false}` (administrator only).

List creation JSON: `{"name":"Groceries","shared":true,"archived":false}`.
Item creation JSON: `{"name":"Milk","quantity":2,"unit":"each","notes":"","purchased":false}`.
Supported units are each, pack, box, bag, lb, oz, kg, g, L and mL.

## Verification before a production release

1. Build and test Go with `go test ./...`.
2. Run `npm install --no-audit --no-fund && npm run build:css && npm run test:web` on the full checkout.
3. Apply both additive migrations to a **restored copy** of a production database and test upgrade + rollback using existing Calden backup tooling.
4. Exercise every route and permission boundary with admin, member, restricted, private-list owner and unauthorised reader; test list hiding, unarchiving and concurrent edits.
5. Test browser accessibility, narrow screens, appearance choices, cross-device dashboard persistence and all existing calendar/bill/notification screens.
6. Validate the Android client's compatibility with the changed settings JSON and verify notification delivery on real devices.
7. Only after acceptance: update VERSION, web cache keys, changelog, build images and create a signed-off release.

## Deferred approved expansion

The remaining family platform capabilities are still governed by [FAMILY-PLATFORM-EXPANSION.md](../FAMILY-PLATFORM-EXPANSION.md). This integration candidate does **not** claim child guardian workflows, rewards/allowance ledger, school records, routines, maintenance, pet care, external-calendar live synchronization, RecipeCove two-way API support, household-device integration or native Android dashboard parity.
