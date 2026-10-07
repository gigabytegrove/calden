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
3. asks for the original Google export
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

If the original export does not provide enough information to identify which stored event should survive, that duplicate group is left unchanged and reported for review.

Backups are still recommended before any bulk repair operation.
