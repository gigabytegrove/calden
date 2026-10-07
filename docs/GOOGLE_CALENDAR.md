# Google Calendar migration and duplicate repair

CalDen has two different Google Calendar workflows. They solve different problems and should not be mixed together.

## Importing a Google Calendar export

Use **Integrations → Google Calendar → Review calendars** when bringing Google Calendar data into CalDen for the first time.

CalDen:

1. reads the Google Takeout ZIP or an ICS file
2. compares exported calendars with existing CalDen calendars
3. lets the administrator explicitly map, skip, or create each destination
4. imports the selected events only after that mapping is confirmed

Google Takeout can contain more than one source record with the same event UID after an event has moved between calendars. Google Calendar itself does not need to show duplicate visible events for this to happen.

Current CalDen treats those records as one logical event and suppresses stale copies before import.

## Repairing duplicates that are already in CalDen

Older CalDen builds could import those source records as multiple database rows. The result is duplicate events across CalDen even though Google Calendar itself was not duplicated.

Do **not** delete all of the CalDen calendars and start over.

Use **Integrations → Google Calendar → Repair duplicate Google events**.

The repair flow:

1. scans the existing CalDen database for multiple imported parent events sharing the same Google UID
2. shows the duplicate groups and redundant CalDen copies it found
3. asks for the current Google export
4. uses that export only as a reference to determine which stored CalDen copy is canonical/current
5. previews how many groups can be repaired safely
6. requires typed confirmation
7. merges CalDen-side metadata into the surviving event
8. deletes only the redundant event rows

The repair action does **not** import events and does **not** delete calendars.

## Data preserved during repair

When a redundant event row is consolidated into the surviving event, CalDen preserves compatible CalDen-side information including:

- event assignees
- nonduplicate reminders
- notification references
- Bill Pay metadata when the surviving calendar is a Bill Pay calendar
- Bill Pay payment entries and no-balance occurrence state when applicable

## Safety rules

CalDen does not guess when it cannot establish a safe canonical destination.

If the current export does not provide enough information to identify which stored event should survive, that duplicate group is left unchanged and reported for review.

Backups are still recommended before any bulk repair operation.


## Consecutive-day all-day display bug

CalDen 0.1.0-alpha11.1 fixed a separate issue that could look like duplicate imported data even when only one event row existed.

RFC 5545/iCalendar uses an **exclusive** `DTEND`. A one-day all-day event represented as:

```text
DTSTART;VALUE=DATE:20261006
DTEND;VALUE=DATE:20261007
```

means **October 6 only**. October 7 is the exclusive end boundary.

Earlier CalDen calendar rendering treated the end boundary as inclusive, so that single event could appear on both October 6 and October 7. That was a display/range-overlap bug, not necessarily two stored events.

Alpha11.1 uses half-open ranges (`[start, end)`) consistently in the calendar UI and event-range queries. Alpha11.2 additionally prevents the browser from keeping a stale pre-fix `app.js` after an in-place update by disabling caching for application HTML/JS/CSS and using release-specific asset URLs. If a one-day all-day event appears on consecutive dates, update to Alpha11.2 before using the database duplicate repair tool.
