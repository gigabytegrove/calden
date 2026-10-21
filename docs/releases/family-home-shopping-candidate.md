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

## Shopping & Supplies expansion — development candidate (migration 039)

The existing shopping application now supports an optional item category (up to 80 characters), assigned household member, stock-on-hand and reorder threshold (stored separately from the quantity to buy). When stock is at or below the configured threshold, the item is clearly marked as running low. This is a user-entered inventory signal, not automatic inventory depletion or purchasing. The item list still separates needed and purchased items, and the original list privacy model remains in force.

- An item can only be assigned to an active member who can access its list. An owner-private list may only assign its owner. Child assignment also requires an enabled Shopping module on the child's profile.
- Existing item PUT requests that omit the new metadata keys preserve their current values, avoiding accidental data loss by older native clients. An explicit JSON null clears nullable fields; blank categories clear category names.
- Shopping history captures the latest 100 snapshots per list. INSERT and UPDATE snapshots are recorded by a PostgreSQL trigger in the same transaction as the item mutation. Direct item deletion records the final snapshot atomically in one SQL statement; deleting an entire list cascades safely to its items and history. Old entries survive individual item deletion but disappear when their parent list is deleted.
- History follows the same server-side private/shared list access policy. The authenticated actor is recorded for standard API item creation, editing, purchase changes and direct deletion; Calden's existing audit log independently records the corresponding action.
- Categories, assignees and threshold values are not exposed as notification content, and no additional background worker is introduced.

### Acceptance outstanding

Go compilation and full test suite, real PostgreSQL migration rehearsal on a restored backup, user-role and child access regression tests, visual/mobile UI review, and Android compatibility tests must still pass before a stable release. No automatic ingredient imports, supply adjustment on purchase, or external inventory changes are claimed.

The shopping interface also includes category filtering and low-stock counts in list summaries. Filters are only a view choice; changing categories never deletes or edits items.

When the stocked quantity is explicitly cleared, the reorder threshold is also cleared. Setting a new reorder threshold requires the client to send a current stock value in the same request. Legacy clients that omit both keys retain both existing values.

Quantities below 0.001 are rejected, matching the database's three-decimal storage precision and preventing a positive user value from silently rounding to zero.
