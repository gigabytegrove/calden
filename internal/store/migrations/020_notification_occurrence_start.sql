-- Persist the recurring instance associated with a notification. Earlier
-- migrations added confirmation notifications but did not add this column,
-- causing occurrence updates and inbox queries to fail on clean installs.
ALTER TABLE notifications ADD COLUMN IF NOT EXISTS occurrence_start TIMESTAMPTZ;
CREATE INDEX IF NOT EXISTS idx_notifications_event_occurrence
    ON notifications(event_id, occurrence_start)
    WHERE occurrence_start IS NOT NULL;
