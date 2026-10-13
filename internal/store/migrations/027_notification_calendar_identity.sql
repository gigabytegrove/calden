-- Retain the calendar identity in notification records even when the event is
-- cancelled or deleted. Enables per-calendar device settings on cancellations.
ALTER TABLE notifications ADD COLUMN calendar_id UUID;

UPDATE notifications n SET calendar_id=e.calendar_id
FROM events e WHERE n.event_id=e.id AND n.calendar_id IS NULL;

CREATE OR REPLACE FUNCTION calden_notification_calendar() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.calendar_id IS NULL AND NEW.event_id IS NOT NULL THEN
        SELECT e.calendar_id INTO NEW.calendar_id FROM events e WHERE e.id=NEW.event_id;
    END IF;
    RETURN NEW;
END $$;

CREATE TRIGGER calden_notification_calendar_before_insert
BEFORE INSERT ON notifications
FOR EACH ROW EXECUTE FUNCTION calden_notification_calendar();

CREATE TRIGGER calden_notification_calendar_before_event_change
BEFORE UPDATE OF event_id ON notifications
FOR EACH ROW EXECUTE FUNCTION calden_notification_calendar();

CREATE INDEX idx_notifications_calendar ON notifications(calendar_id)
WHERE calendar_id IS NOT NULL;
