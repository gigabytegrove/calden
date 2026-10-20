# Calden — Child Accounts, Guardians & Family Layouts

**Status: integrated development candidate; not production-accepted and not released.** This extends existing Calden users, calendars, chores, routines, shopping and rewards. The published stable version remains 1.4.8.

## Delivered behavior

- Household administrators can create an explicitly designated **child account** in People -> Child accounts, with a unique sign-in, password, simplified or standard layout, permitted family modules and at least one active adult guardian.
- Existing restricted-member accounts are not silently converted. New child accounts carry the existing restricted role plus an explicit `users.is_child` flag; a database check prevents upgrading a child to administrator/member/planner while the child flag remains true.
- Administrators can change a child's module permissions, dashboard layout and assigned adult guardians; a child cannot edit their own permissions or other household member accounts.
- Active, designated adult guardians can see which children are assigned to them via `GET /api/family/children`. Administrator authority is still required to change guardian assignments or permissions in this candidate.
- Children can sign in normally, see a simpler home and use permitted personal features. Their calendar shows authorized standard events, never Bill Pay calendars. The personal dashboard excludes Bill Pay and modules their guardian has not authorized.
- The backend validates child permissions on *every* authenticated API request against current PostgreSQL data, not cached JWT roles. All unspecified routes and operations are denied for child accounts.
- Child chore lists show only chores assigned to that child; restricted children can complete only their own chore occurrences and routine steps. Reward balances/claims are personal; child access does not grant point adjustments, allowance payments or approval rights.
- Shopping, when enabled for a child, is read-only; other household members' private lists remain hidden.
- Existing adult accounts and household settings are unchanged. No extra service or separate child database.

## Schema and API

Migration **033_child_guardian_profiles.sql** adds a non-destructive child flag, restricted-role constraint, child profile module/layout settings and an indexed child-to-guardian link table.

All child management endpoints are authenticated:

- `GET /api/family/children` — administrator gets all profiles; designated guardians see assigned profiles.
- `POST /api/family/children` — administrator only; atomically creates a restricted child and at least one guardian.
- `PUT /api/family/children/{id}` — administrator only; atomically updates allowed modules, layout and guardian assignments.
- `GET /api/me` — now includes `is_child`, `child_modules` and `child_layout` for child accounts.
- `GET/PUT /api/me/dashboard` — normalized and validated against guardian-controlled layout and modules for children.

A sample child-creation request:

```json
{
  "username":"alex",
  "display_name":"Alex",
  "password":"provide-a-unique-private-password",
  "display_mode":"simple",
  "allowed_modules":["calendar","chores","routines","rewards"],
  "guardian_ids":["a-real-active-adult-uuid"]
}
```

Supported module grants: `calendar`, `chores`, `tasks`, `routines`, `rewards`, `shopping`, `notifications`, `dashboard`. No grant unlocks bills, account management, financial adjustments or household integrations.

## Security boundaries

1. The authenticated request reads the current role, activity status and child flag from PostgreSQL, then loads that child's current grant list and denies non-allowlisted operations.
2. The REST event list, calendar list and replacement-occurrence event queries filter Bill Pay calendars for child accounts even if Bill Pay calendar permissions were mistakenly assigned.
3. Both the REST notification inbox and the live notification WebSocket apply viewer calendar access and child Bill Pay exclusions.
4. Dashboard editing rejects disallowed widgets and guardian-locked layout changes; reading sanitizes previously saved dashboard settings.
5. Child user discovery does not return other members' usernames. It retains display names needed for household collaboration.
6. Child accounts have no authority over guardians, permissions, other profiles, rewards ledgers, integrations, payment recording or backend admin settings.
7. Guardians assigned to a child can view that child's profile configuration. Delegated guardian approval workflows and full native Android child UI are **future work**, not implied by this candidate.

## Verification before publishing a stable release

- Run `go test ./...` and the project's complete web tests. New tests cover grant validation, default-deny routing, per-user dashboard filtering, UI wiring and calendar/notification confidentiality clauses.
- Apply migration 033 on an isolated restored copy of the existing database; verify upgrade, backup/restore, and compatibility with migrations 029–032.
- Test admin, planner, member, assigned guardian, unrelated adult, child and deactivated accounts using real HTTP requests, including request forgery/ID guessing.
- Test child login, dashboard load, unsupported feature removal, permission revocation during a session, calendar sharing, recurring-event overrides, notification WebSocket and push delivery.
- Test child accounts on narrow screens, keyboard-only navigation and native Android, particularly when calendar access is disabled.
- Complete the existing stable release acceptance list before tagging or deploying.

See [approved product scope](../FAMILY-PLATFORM-EXPANSION.md) for subsequent school, care, theme and integrations work.
