# Google Calendar migration and duplicate repair

CalDen has two separate responsibilities: importing Google Calendar data correctly, and cleaning up duplicate event rows that older CalDen imports may already have created.

## Importing Google Calendar

Use **Integrations → Google Calendar → Review calendars** for a normal import.

CalDen previews the exported calendars, lets the administrator map each source to the intended CalDen calendar, and imports only after that mapping is confirmed.

Google UIDs remain the primary import identity, but CalDen no longer assumes that every visible duplicate will necessarily share one UID.

## Repairing duplicates already stored in CalDen

Older builds could leave more than one stored CalDen event that represents the same real event. That can happen even when Google Calendar itself shows only one event.

The repair tool works against **CalDen's stored data**, not by blindly trusting an export to decide which copy is correct.

Use **Integrations → Google Calendar → Repair duplicate imported events**.

The workflow is:

1. Scan the imported event rows already stored in CalDen.
2. CalDen groups suspicious copies by imported title, recurrence shape, duration, and calendar context.
3. Each stored copy is shown with its actual date and CalDen calendar.
4. **You choose which copy to keep.**
5. Groups can be skipped entirely.
6. CalDen requires typed confirmation before deleting anything.
7. CalDen merges compatible CalDen-side metadata into the selected survivor and removes only the copies you explicitly selected.

No calendar is deleted. The repair tool does not re-import the Google export.

## Why the administrator chooses the survivor

Google's current state and an older Takeout export can disagree, and changed Google event identities can leave two CalDen rows with different UIDs.

Because of that, CalDen does not infer that an earlier or later date is automatically correct. It can flag a suspicious 6th/7th pair, for example, but the administrator chooses whether the 6th or 7th survives.

The interface also identifies the newest CalDen row as a convenience, but that is only a suggestion; it is not selected automatically.

## Data preserved during repair

When redundant rows are consolidated, CalDen preserves compatible CalDen-side information including:

- event assignees
- nonduplicate reminders
- notification references
- Bill Pay metadata when the surviving event is on a Bill Pay calendar
- Bill Pay payment records and no-balance occurrence state when applicable

## Safety

The resolver validates the selected group again inside the database transaction before removing anything. If the group changed after the scan, CalDen refuses the repair and asks for a fresh scan.

Backups are still recommended before any bulk cleanup.
