# Calden

Calden is a self-hosted family calendar designed around people, shared calendars, clear ownership, and simple reminders.

## Current development build

Version: `0.1.0-alpha1`

The first build includes:

- Docker deployment with PostgreSQL
- First-run administrator setup
- User accounts with initials/avatar-ready identity
- Admin-created calendars with definitive colors
- Per-calendar visibility and edit permissions
- Events assigned to one or more people
- Personal and system reminder records
- Responsive web UI
- REST API foundation for Calden for Android and integrations
- Health endpoint

## Run

```bash
cp .env.example .env
docker compose up -d --build
```

Open `http://localhost:8787`.

On a new installation Calden shows the first-run setup screen and creates the first administrator.

## Design rules

- The calendar determines an event's color.
- Assigned people are shown by their identity/avatar.
- Admins decide who can see and edit each calendar.
- Personal reminders and household/system reminders are separate.
- User-facing screens use plain language. Protocol and implementation terminology stays out of normal UI.

## Repositories

- Server/web: `gigabytegrove/calden`
- Android: `gigabytegrove/calden-android`
