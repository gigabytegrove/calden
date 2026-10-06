# CalDen

**A family calendar and scheduling system you control.**

CalDen is a self-hosted family calendar built for households, not calendar administrators. It gives everyone one simple place to see what is happening, which calendar an event belongs to, and who is responsible for it.

Create calendars for things like **Family, Bills, School, Appointments, Work, Birthdays, Vacations, Sports, or anything else your household needs**. Give each calendar its own color, decide who can see it, decide who can change it, and assign individual events to one or more people.

CalDen includes a responsive web app, a native Android companion app, normal Android reminders, and household-wide reminders through [Monita](https://github.com/gigabytegrove/monita).

> **Development status:** CalDen is under active development. The current `0.1.x` series is a rolling alpha: it is being built as a usable product, but backups are strongly recommended before updates.

---

## The CalDen idea

CalDen deliberately keeps the visual rules simple:

| What you see | What it means |
| --- | --- |
| Event color | The calendar the event belongs to |
| Person avatar or initials | Who the event is assigned to |
| Calendar visibility | Who can see that calendar |
| Calendar editing access | Who can add or change events there |
| Phone reminder | A personal Android reminder |
| Monita reminder | A household/system reminder |

If you create a green **Bills** calendar, every bill appears as a green event. The person responsible for each bill is shown separately by their avatar or initials.

That means the calendar can stay shared without losing clear ownership.

---

## What CalDen can do

### People

Each family member gets their own CalDen account.

CalDen currently supports administrator, family-member, and restricted-member accounts. Each person has their own name and initials, with richer avatar/photo support continuing to expand as the app develops.

### Calendars

Administrators create the household calendars and choose:

- the calendar name;
- its color;
- its description;
- who can see it;
- who can make changes.

A calendar can be shared with the whole family, shared with only selected people, or effectively private to one person.

### Events

Events can include:

- a title;
- start and end times;
- location;
- notes;
- a selected calendar;
- one or more assigned people;
- a personal phone reminder;
- a household reminder through Monita.

Events can be created, edited, and deleted from the web interface. The same event and assignment model is used by the Android app.

### Personal Android reminders

CalDen for Android can schedule normal Android notifications for reminders assigned to the user.

These are intentionally separate from family-wide reminders. A person can receive a personal reminder without notifying the whole household.

### Monita family reminders

CalDen can connect to a Monita server and send system-wide reminders for events.

For example, an electric bill could create:

> **Electric bill**  
> Due tomorrow

while a family appointment could create:

> **Family doctor appointment**  
> Wednesday at 2:00 PM · Valdosta

CalDen tracks Monita delivery attempts and retries failed system reminders instead of making the calendar depend on Monita being online every second.

---

# Installing CalDen

CalDen is designed to run in Docker.

You need a Linux server, VM, NAS, or other machine capable of running Docker and Docker Compose. PostgreSQL is included in the supplied Compose configuration, so a separate database installation is not required.

There are **two supported Compose styles**. Pick the one that matches how you are deploying.

## Option 1 — Clone the repository and build locally

Use this when the CalDen repository exists on the Docker host.

```bash
git clone https://github.com/gigabytegrove/calden.git
cd calden
cp .env.example .env
```

Edit `.env` and replace the example database password:

```env
CALDEN_PORT=8787
CALDEN_DB_NAME=calden
CALDEN_DB_USER=calden
CALDEN_DB_PASSWORD=CHANGE-ME-TO-A-LONG-RANDOM-PASSWORD
```

Then start CalDen:

```bash
docker compose up -d --build
```

The repository's normal `compose.yaml` uses the included Dockerfile and builds CalDen from the checked-out source.

## Option 2 — Raw Compose / Dokploy / Portainer / Dockge

Use this when your deployment platform accepts Compose text but **does not clone the CalDen source repository into the deployment directory**.

Use:

```text
docker-compose.example.yml
```

That file pulls the published CalDen container instead of using `build: .`.

The important difference is:

```yaml
calden:
  image: ghcr.io/gigabytegrove/calden:latest
```

A raw Compose deployment must use the image-based example. A source-build Compose file requires a local Dockerfile and source tree and will fail with an error such as:

```text
failed to read dockerfile: open Dockerfile: no such file or directory
```

when used in a raw-Compose deployment.

### Environment values for raw Compose

At minimum, set:

```env
CALDEN_DB_PASSWORD=CHANGE-ME-TO-A-LONG-RANDOM-PASSWORD
```

Optional values are:

```env
CALDEN_PORT=8787
CALDEN_DB_NAME=calden
CALDEN_DB_USER=calden
```

If your GitHub Container Registry package requires authentication during the alpha period, configure your deployment platform with GitHub Container Registry credentials that have permission to read the CalDen package.

---

# First start

Once the containers are healthy, open:

```text
http://YOUR-SERVER-IP:8787
```

For example:

```text
http://192.168.1.50:8787
```

The first browser to open a new CalDen installation receives the setup screen.

Create the first administrator account there. No default CalDen username or password is created.

After signing in, use **Manage CalDen** to add your family members and create the calendars you want to use.

A good starting set might be:

- Family;
- Bills;
- Appointments;
- School;
- Birthdays;
- Work.

You are not required to use any particular calendar layout. CalDen is intended to fit the household rather than force the household into a fixed structure.

---

# Setting up a calendar

When creating a calendar, choose its color carefully because that color becomes the visual identity of every event on that calendar.

Then select:

**Who can see it?**  
These people can view the calendar and its events.

**Who can make changes?**  
These people can add and edit events on the calendar.

The interface intentionally uses those phrases instead of technical permission terminology.

---

# Adding an event

Choose **Add event** and enter the event information.

Select the calendar first, then choose the person or people the event is for.

For example:

```text
Electric Bill
Calendar: Bills
Assigned to: Brad
Phone reminder: 1 day before
Family reminder: 3 days before
```

The event uses the Bills calendar color while Brad's identity shows who is responsible for it.

An event can also be assigned to several people when responsibility is shared.

---

# Connecting Monita

Administrators can connect CalDen to Monita from **Manage CalDen → Monita**.

Enter:

- the Monita server address;
- the token CalDen should use;
- the default channel;
- whether Monita reminders are enabled.

Use **Send test** to confirm the connection.

Once connected, an event can include a **family reminder** in addition to a personal Android reminder.

CalDen's reminder worker checks for due system reminders in the background and keeps delivery state so temporary Monita failures can be retried.

---

# CalDen for Android

The Android client lives in the separate [calden-android](https://github.com/gigabytegrove/calden-android) repository.

The app connects directly to your CalDen server.

On first sign-in, enter your CalDen server address and your normal CalDen username and password.

The Android app is being developed alongside the server and uses the same model:

- calendar color identifies the calendar;
- avatars/initials identify assigned people;
- personal reminders use normal Android notifications;
- shared system reminders remain a server/Monita function.

---

# Data and backups

CalDen's supplied Compose configurations use two persistent Docker volumes:

```text
calden-db
calden-data
```

**calden-db** contains the PostgreSQL database.

**calden-data** contains CalDen's persistent application data, including the locally generated session-signing secret.

Both should be included in your normal Docker backup routine.

Do not treat the container itself as the backup. Containers are disposable; the persistent volumes are the important part.

---

# Updating

## Source-build install

From the CalDen directory:

```bash
git pull --ff-only
docker compose up -d --build
```

## Published-image / raw Compose install

Pull the current image and recreate the containers:

```bash
docker compose pull
docker compose up -d
```

Database migrations run automatically when CalDen starts.

Back up your persistent data before major upgrades or while running alpha releases.

---

# Changing the web port

CalDen listens on port `8787` inside the container.

To expose it on another host port, change only:

```env
CALDEN_PORT=9000
```

Then open:

```text
http://YOUR-SERVER-IP:9000
```

The container itself continues listening on `8787`. This avoids mismatched Docker port mappings.

---

# Reverse proxy and HTTPS

CalDen can sit behind a normal reverse proxy such as NGINX, NGINX Proxy Manager, Caddy, Traefik, or another HTTPS-capable proxy.

Proxy the public hostname to:

```text
http://CALDEN-SERVER:8787
```

For use outside your trusted home network, HTTPS is strongly recommended.

Do not expose PostgreSQL to the internet. The supplied Compose examples do not publish the database port.

---

# Health check

CalDen exposes:

```text
GET /api/health
```

A healthy server returns a response containing:

```json
{
  "ok": true,
  "version": "0.1.0-alpha1"
}
```

The Docker Compose examples use this endpoint to check the application container.

---

# Troubleshooting

## `open Dockerfile: no such file or directory`

You are using a source-build Compose file in a raw-Compose deployment.

Use `docker-compose.example.yml`, which pulls the CalDen image instead.

## Database does not become healthy

Check that `CALDEN_DB_PASSWORD` exists and that the same value is being passed to both the database and CalDen services.

Then check:

```bash
docker compose logs db
```

## CalDen does not start

Check:

```bash
docker compose logs calden
```

Then verify container state:

```bash
docker compose ps
```

## The published image cannot be pulled

If Docker reports `denied` or `unauthorized`, the registry requires authentication for that deployment.

Configure your Docker host or deployment platform for `ghcr.io` with GitHub credentials that can read the CalDen container package, then deploy again.

## Monita test fails

Verify that:

- the Monita server address is reachable from the CalDen container;
- the token is correct;
- the Monita server accepts the configured message route;
- the selected/default channel exists.

A Monita outage does not prevent normal CalDen calendar use.

---

# Configuration reference

| Setting | Default | Purpose |
| --- | --- | --- |
| `CALDEN_PORT` | `8787` | Host port used by the supplied Compose files |
| `CALDEN_DB_HOST` | `localhost` outside Docker | PostgreSQL host |
| `CALDEN_DB_PORT` | `5432` | PostgreSQL port |
| `CALDEN_DB_NAME` | `calden` | Database name |
| `CALDEN_DB_USER` | `calden` | Database user |
| `CALDEN_DB_PASSWORD` | development fallback only | Database password |
| `CALDEN_DB_SSLMODE` | `disable` | PostgreSQL SSL mode |
| `CALDEN_DATABASE_URL` | unset | Optional complete PostgreSQL URL override |
| `CALDEN_DATA_DIR` | `./data` outside the image | Persistent CalDen data directory |
| `CALDEN_WEB_DIR` | `./web` outside the image | Web interface files |

For normal Docker installs, use the supplied Compose variables instead of manually building a database URL.

---

# Development

The CalDen server is written in Go and serves the web interface directly.

The main application repository contains:

```text
cmd/calden/          CalDen server entry point
internal/api/        Web/API handlers
internal/store/      PostgreSQL connection and migrations
internal/reminders/  Background reminder delivery
web/                 Responsive browser interface
```

The Android application is maintained separately in `gigabytegrove/calden-android`.

Every push to `main` runs server checks. The container publishing workflow builds CalDen for supported Docker architectures and publishes the rolling `latest` image when the build passes.

---

# Product principles

CalDen is intentionally opinionated about usability:

1. A family member should not need to understand calendar standards to use the calendar.
2. Calendar color and event ownership must remain visually distinct.
3. Sharing must be described in plain language.
4. Personal notifications and household notifications must remain separate concepts.
5. The server should continue functioning when an optional integration is unavailable.
6. Administration can be powerful without leaking technical terminology into normal family use.

If a normal household user has to know what a protocol name or backend acronym means to complete an everyday task, that part of CalDen needs a better interface.
