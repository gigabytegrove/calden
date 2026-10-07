ALTER TABLE events
    ADD COLUMN external_uid TEXT;

CREATE UNIQUE INDEX idx_events_external_uid_calendar
    ON events(calendar_id, external_uid)
    WHERE external_uid IS NOT NULL AND recurrence_parent_id IS NULL;
