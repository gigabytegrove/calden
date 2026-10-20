<p align="center">
  <img src="web/calden-logo-official.webp" alt="Calden" width="520">
</p>

<h1 align="center">Calden</h1>

<p align="center"><strong>A family calendar and scheduling system you control.</strong></p>

Calden is a self-hosted family calendar built for normal people, not calendar administrators. It keeps household schedules in one place, makes it obvious which calendar an event belongs to, and shows exactly who an event is assigned to.

A calendar has its own color. A person has their own identity. The two never compete with each other.

Calden 1.2.0 is the first stable release. Development continues on the existing web application, with compatible in-place updates.

## Unreleased family expansion development

The approved family platform expansion is now being implemented on the existing application. A development candidate adds personal dashboard configuration and household shopping lists with shared/private access, persistent items, feature visibility and a responsive browser interface. **These changes are not part of the 1.4.8 stable release and are not yet production accepted.** See [implementation and acceptance notes](docs/releases/family-home-shopping-candidate.md) and [the full approved scope](docs/FAMILY-PLATFORM-EXPANSION.md). Android parity, integration tests and remaining family modules are still required before announcing the full expansion as released.

## Product direction

The approved product scope, implementation priorities, and compatibility boundaries are documented in [Product Direction](docs/PRODUCT-DIRECTION.md). Proposed capabilities are not represented as released until implemented and tested.

## What Calden does

Calden gives a household one shared place to manage schedules without forcing every calendar to be shared with everybody.

You can use it to:

- create separate calendars for things like Family, Bills, School, Work, Appointments, Birthdays, and Vacations
- choose exactly who can see each calendar
- choose exactly who can make changes to each calendar
- assign an event to one or more people
- tell who is responsible for an event by looking at the person's icon
- tell which calendar an event belongs to by looking at the event color
- give people their own sign-in
- use reminder records for personal-device notifications
- send household-wide reminders through Monita
- use Calden from a browser on desktop, tablet, or phone
- use the same Calden server/API as the foundation for the upcoming mobile app

## How Calden is organized

Calden follows three simple rules:

**Calendar = where the event belongs**

The calendar decides the event color.

**Person = who the event belongs to**

The assigned person's icon or initials appear with the event.

**Reminder = how people are notified**

Personal reminders can stay on the person's phone. Household reminders can be sent through Monita.

For example, a green **Bills** calendar can contain an Internet Bill assigned to Brad and an Electric Bill assigned to Jen. Both events stay green because they belong to Bills, while the person icon shows who is responsible for each one. Calendar bill status markers show allocated, paid, and cleared bills at a glance. Bill activity is tracked per occurrence: funds can be marked **Allocated** when money is moved into a bill-pay account, then **Paid** when the withdrawal actually happens, and **Cleared** after it settles. A month can also have no balance or several split/partial payments, each with its actual payer, payment date, and cleared date. Calden creates an end-of-day bill follow-up for open bills so the assigned payer can confirm the autopay came out.

## Current features

The current development build includes:

- guided first-run setup wizard
- automatic persistent database credential generation
- family name and time-zone setup
- starter calendar selection
- multiple user accounts
- administrator, family member, and restricted member account types
- multiple calendars
- Bill Pay calendars with expected amount, assigned payer, allocated-funds tracking, no-balance months, partial/split payments, actual payer, payment date, cleared state, end-of-day payment follow-ups, and household breakdowns
- calendar colors
- calendar visibility controls
- calendar editing permissions
- typed destructive deletion of calendars that still contain events
- permanent category deletion; events using a deleted category become uncategorized
- multi-person event assignment
- persistent assignment notifications and whole-family event alerts
- optional live browser notifications with unread inbox badges
- event creation
- event editing
- event deletion
- event notes and locations
- real 1 Day, 7 Day, 14 Day, and 30 Day calendar views
- recurring events with single-occurrence changes
- standard iCalendar import/export
- Google Calendar ZIP migration with preview and explicit mapping, plus an in-place repair tool for duplicate Google-imported event rows already stored in Calden
- profile images for family members
- personal-device reminder records
- Monita household reminder records
- Monita connection settings
- Monita test notifications
- background Monita reminder delivery
- reminder delivery retry tracking
- Tailwind CSS v4-backed responsive web interface with a polished application shell
- PostgreSQL storage
- automatic database migrations
- Docker deployment
- health checks

Calden is being developed as a rolling product. Features are being added to the working application rather than held for a future rewrite.

The production web bundle is built with Tailwind CSS v4. Docker and release builds compile it automatically. For direct source development outside Docker, run:

```bash
npm install
npm run build:css
```

## Quick install with Docker Compose

### 1. Download Calden

```bash
git clone https://github.com/gigabytegrove/calden.git
cd calden
```

### 2. Start Calden

Calden does not require you to create a database password or edit an environment file for a normal installation. On the first start, Docker generates a private random database password and keeps it in the persistent `calden-secrets` volume.

```bash
docker compose up -d --build
```

### 3. Open Calden

By default:

```text
http://YOUR-SERVER-IP:8787
```

The first time Calden opens, a guided setup wizard walks you through your family name, time zone, first administrator account, and starter calendars. When setup finishes, Calden opens with useful calendars already in place.

You can then add family members, change calendar access, connect Monita, and start scheduling.

## Docker Compose example

A complete copy-and-run example is included as:

```text
docker-compose.example.yml
```

The repository also includes the normal source deployment file:

```text
compose.yaml
```

For Docker platforms that only accept pasted Compose and do **not** clone the repository, use:

```text
docker-compose.dokploy.yml
```

That version builds Calden directly from the public GitHub repository. Dokploy does not need a local checkout, a GitHub Container Registry login, or a manually configured database password.

## Dokploy

Calden supports both normal Git deployments and Docker Raw deployments in Dokploy.

### Recommended: Git deployment

Point Dokploy at:

```text
https://github.com/gigabytegrove/calden
```

Use the repository Compose file. Because Dokploy has the repository in this mode, `build: .` can see the Calden Dockerfile and source code.

### Docker Raw deployment

If you create a **Docker Raw** application and paste Compose directly into Dokploy, do not use the source-build Compose file.

Use:

```text
docker-compose.dokploy.yml
```

It builds directly from:

```text
https://github.com/gigabytegrove/calden.git#main
```

The Dockerfile comes from that public Git repository, so no GHCR credentials are required.

No environment variables are required for a new Dokploy installation. The database password is generated automatically and persisted in the `calden-secrets` volume.

You can optionally change the exposed port:

```env
CALDEN_PORT=8787
```

## Updating

### Source deployment

From the Calden directory:

```bash
git pull --ff-only
docker compose up -d --build
```

Calden runs database migrations automatically when the updated container starts.

### Docker Raw / Dokploy deployment

Redeploy the application. Docker rebuilds Calden from the current public `main` branch and reuses the existing data volumes.

No registry login is required.

## Backups

A complete Calden backup has three important parts:

- the PostgreSQL database
- the Calden data volume
- the Calden secrets volume

The database contains users, calendars, events, permissions, reminders, and settings.

The Calden data volume contains server-generated application data such as the persistent signing secret. The `calden-secrets` volume contains the automatically generated PostgreSQL password and must be preserved with the database.

For a manual PostgreSQL backup:

```bash
docker compose exec -T db pg_dump -U calden -d calden > calden-backup.sql
```

Also back up the Docker volume named:

```text
calden-data
```

For a complete container-level backup, preserve all three named volumes:

```text
calden-db
calden-data
calden-secrets
```

Do not remove the data volumes during a normal update.

## Monita reminders

Calden can use Monita for household-wide reminders.

In Calden:

1. Open **Manage**
2. Find **Monita**
3. Enter the Monita server address
4. Enter the token Calden should use
5. Choose the default channel
6. Turn on Monita reminders
7. Send a test

An event can then have both:

- a personal phone reminder
- a family reminder through Monita

These are separate on purpose. The person responsible for an event can get a private phone reminder while the rest of the household still receives the system-wide reminder.

If Monita is unavailable when a system reminder is due, Calden records the failed delivery and retries it instead of taking the calendar offline.

## Mobile app

The native Android application is developed in [gigabytegrove/calden-android](https://github.com/gigabytegrove/calden-android) alongside this server. Calden 1.3.6 supports Android's home-screen calendar and agenda widgets, date-aware quick event creation, per-user week start preferences, the Planner role, confirmations, and the per-user notification inbox.

Android source version 1.3.6 is available. APK builds must be compiled and signature-verified before installation; the presence of source or a tag alone does not prove that an APK is ready.

## Ports and storage

The default browser port is:

```text
8787
```

The container itself always listens on port `8787`. `CALDEN_PORT` changes only the host-side port exposed by Docker Compose.

Persistent Docker volumes:

```text
calden-db
calden-data
calden-secrets
```

PostgreSQL is not published to the host by default.

## Reverse proxy and HTTPS

If Calden is exposed outside your home network, put it behind an HTTPS reverse proxy.

Forward the proxy to:

```text
http://CALDEN-SERVER:8787
```

or to whichever host port you selected with `CALDEN_PORT`.

Calden does not require a special URL path. Hosting it at the root of a hostname is the simplest setup, for example:

```text
https://calendar.example.com
```

## Health check

Calden exposes:

```text
/api/health
```

A healthy server returns an `ok` response and the running version.

Docker Compose uses this endpoint to confirm that the application is responding.

## Environment settings

A normal Docker installation needs no environment settings. The options below are available when you want to customize the deployment.

| Setting | Default | Purpose |
| --- | --- | --- |
| `CALDEN_PORT` | `8787` | Port exposed on the Docker host |
| `CALDEN_DB_NAME` | `calden` | PostgreSQL database name |
| `CALDEN_DB_USER` | `calden` | PostgreSQL username |
| `CALDEN_DB_PASSWORD` | automatically generated | Optional manual PostgreSQL password override |
| `CALDEN_DB_HOST` | `localhost` outside Compose | Database hostname |
| `CALDEN_DB_PORT` | `5432` | Database port |
| `CALDEN_DB_SSLMODE` | `disable` | Database TLS mode |
| `CALDEN_DATA_DIR` | `/data` in Docker | Persistent application-data location |
| `CALDEN_DATABASE_URL` | unset | Advanced override for the complete PostgreSQL connection string |

If `CALDEN_DATABASE_URL` is supplied, it takes priority over the individual database settings.

## Troubleshooting

### "failed to read dockerfile: open Dockerfile: no such file or directory"

The local-source Compose file was started somewhere that does not contain the Calden repository.

This commonly happens when `compose.yaml` is pasted into a Dokploy **Docker Raw** application.

Use `docker-compose.dokploy.yml`. It fetches the public Calden repository as the Docker build context and does not depend on a local Dockerfile.

### "unauthorized" when pulling ghcr.io/gigabytegrove/calden

Older Docker Raw examples used the GitHub Container Registry image. GHCR package visibility can require authentication even when the source repository itself is public.

Current Calden Docker Raw deployments do not use GHCR. Replace the old Compose with `docker-compose.dokploy.yml` and redeploy.

### Database password

New installations generate the database password automatically. It is stored in the persistent `calden-secrets` volume and reused across restarts and upgrades.

If you are migrating an older installation that already has a manually configured PostgreSQL password, set `CALDEN_DB_PASSWORD` to that existing password before the first start of the migrated Compose stack.

### The database is not ready yet

The Calden container waits for PostgreSQL's health check before starting. On the first launch PostgreSQL may need several seconds to initialize.

Check:

```bash
docker compose ps
docker compose logs db
docker compose logs calden
```

### I changed the browser port and Calden stopped responding

Do not change the application's internal container port.

Set only:

```env
CALDEN_PORT=YOUR-PORT
```

The Compose file maps that host port to Calden's internal port `8787`.

## Development status

Calden's latest stable server release is **1.4.7**. You can install updates from the built-in updater. Back up your family calendar before installing an update.

This release improves wording and setup guidance without changing appointments, bills, or household settings. Server upgrades run database migrations when necessary; version 1.4.4 does not add a database migration.

Current stable version:

```text
1.4.7
```

## Project layout

```text
calden/
├── cmd/                 Calden server entry point
├── internal/            Server, storage, reminder, and integration code
├── web/                 Calden web interface
├── Dockerfile
├── compose.yaml
├── docker-compose.example.yml
└── docker-compose.dokploy.yml
```

## License

Calden is under active development. The repository's license file governs redistribution and use.

## Navigable URLs and update caching

Main pages have real paths (`/calendar`, `/bills`, `/agenda`, `/people`, `/settings`), and individual events and bills can be linked as `/events/{id}?at={occurrence}` and `/bills/{id}?at={occurrence}`. The event details dialog has **Copy link**. Browser Back/Forward and deep links after login restore the view, subject to each account's permissions. Calden now serves all static UI files with no-store headers and compares the running version against the browser shell so a completed update automatically refreshes the interface without Ctrl+F5.

## Calendar, agenda and household reminders (1.2.0)

Bill Pay calendar entries use a gold hourglass for allocated funds and one green check for paid or cleared bills. Bill rows show payer avatars when available, not long payment-state labels. Event assignee avatars are unchanged. On wider screens the day/week timeline adapts to viewport height; dense and smaller displays may still scroll.

Agenda defaults to upcoming 30 days, with 7, 14, 30 and 90 day windows plus type and calendar filtering. Notification inbox entries may be dismissed per household user; unread flags and dismissal are separate.

Personal phone reminders can target one named member independent of the event's assignees. An empty reminder recipient targets the event assignees, or the whole household for unassigned events. The Android app must be updated and signed into that person's account to schedule local notifications; it synchronizes in the background approximately every 15 minutes subject to Android restrictions. This is device-scheduled notification delivery, not immediate Firebase server push. For events created shortly before reminder time, a phone that has not yet synced may miss that reminder. Exact alarm permission can improve timeliness where granted.

## Settings and updater (1.2.1)

Integrations, Updates and Backups are now Settings subpages at `/settings/integrations`, `/settings/updates`, and `/settings/backups`. Previous direct links continue to resolve. While an update is running, progress is visible; after completion, Calden shows the result and moves the completed steps into a collapsed activity history. Allocated bills show a high-contrast warning-gold badge with a dark monochrome hourglass.

## 1.2.2 Settings stability

Fix an incorrect Settings pane selector that could prevent Calden from loading in the initial 1.2.1 publishing attempt. Install 1.2.2, not the earlier 1.2.1 tag.

## Tasks & Chores and live sync (1.3.8)

Household administrators may enable **Tasks & Chores** from Settings → Household.
The feature starts disabled on existing installations. Turning it off hides the
section without deleting assignments. Household administrators and Planners can
assign chores; assigned members can mark Pending, In progress, or Done.

Server changes to events, calendars, bills and chores now invalidate connected
Android app/widget caches through the authenticated Calden WebSocket, without
sharing private event data. Clients also catch up through REST when reconnecting.
Android's battery and background limits may delay updates when the direct
connection is not running.

The Android 1.3.8 source also adds a **30-day calendar-only widget**, a native
Tasks & Chores screen, and confirmation/error messaging for event saving.

## Tasks & Chores and device inventory (1.4.1)

Tasks & Chores supports recurring household work (daily, weekly, every two weeks, monthly or yearly), optional end dates, multiple assignees, and independent per-occurrence completion for each person. Existing chores migrate in place. Admins can hide Bills and Agenda in Settings → Household without deleting any data. Administrators can review registered devices and their owners under People; individual users still control their own devices from Settings → My devices. See [1.4.1 release notes](docs/releases/v1.4.1.md).

## Appointment date reliability (1.4.3)

Calden keeps the selected civil date intact when opening an event from a calendar date, confirms the PostgreSQL start and end timestamps after creation or updates (including recurring occurrence edits), and navigates to the saved date. The Notifications inbox explicitly identifies its timestamp as **Notified** rather than the appointment date. The January 13, 2027 test case is included in automated browser and API regression tests. See [1.4.3 release notes](docs/releases/v1.4.3.md).

## Getting started as a family

New to Calden? See the [simple family quick-start guide](docs/FAMILY_QUICK_START.md). It explains setup, appointments, calendars, permissions, notifications and updates without technical jargon.

## Calden 1.4.4

This update gives Calden consistent spelling throughout the website and clearer first-time instructions. Your calendars and appointments are not changed. See the [1.4.4 release notes](docs/releases/v1.4.4.md) and the [family quick-start guide](docs/FAMILY_QUICK_START.md).

## Calden 1.4.5 — Optional Features

Household administrators can open **Settings → Optional Features** to independently turn Bills, Agenda, Tasks and Chores on or off for everyone. Calden saves each switch automatically and never deletes the feature's existing data when it is disabled. [Read the 1.4.5 release notes](docs/releases/v1.4.5.md).

## Calden 1.4.6

Fixes the Tasks and Chores navigation icon disappearing when household feature switches change. The labels still reflect what your family has enabled; your saved information is unaffected. [Read the 1.4.6 notes](docs/releases/v1.4.6.md).

## Calden 1.4.7 — Clearer buttons and payment history

Bill payment records now show properly spaced **Mark cleared**, **Edit**, and **Delete** buttons instead of crowded text links. The payment history displays cleanly on narrow screens, and deleting a saved backup now uses a clearly marked button too. No data or payment behavior has changed. [Read the family-friendly release notes](docs/releases/v1.4.7.md).
