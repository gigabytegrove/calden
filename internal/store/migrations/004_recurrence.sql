CREATE TABLE event_recurrence (
    event_id UUID PRIMARY KEY REFERENCES events(id) ON DELETE CASCADE,
    frequency TEXT NOT NULL CHECK (frequency IN ('daily','weekly','monthly','yearly')),
    interval_value INTEGER NOT NULL DEFAULT 1 CHECK (interval_value BETWEEN 1 AND 365),
    weekdays JSONB NOT NULL DEFAULT '[]'::jsonb,
    until_at TIMESTAMPTZ,
    occurrence_count INTEGER CHECK (occurrence_count IS NULL OR occurrence_count > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (until_at IS NULL OR occurrence_count IS NULL)
);

CREATE INDEX idx_event_recurrence_until ON event_recurrence(until_at);

ALTER TABLE reminder_deliveries
    ADD COLUMN occurrence_start TIMESTAMPTZ;

UPDATE reminder_deliveries d
SET occurrence_start = e.starts_at
FROM reminders r
JOIN events e ON e.id = r.event_id
WHERE d.reminder_id = r.id
  AND d.occurrence_start IS NULL;

ALTER TABLE reminder_deliveries
    ALTER COLUMN occurrence_start SET NOT NULL;

ALTER TABLE reminder_deliveries
    DROP CONSTRAINT reminder_deliveries_pkey;

ALTER TABLE reminder_deliveries
    ADD PRIMARY KEY (reminder_id, occurrence_start);

CREATE INDEX idx_reminder_deliveries_status ON reminder_deliveries(status, occurrence_start);
