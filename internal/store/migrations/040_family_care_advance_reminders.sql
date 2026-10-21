-- 040: per-schedule advance notices for the existing school, home upkeep,
-- and pet-care reminder pipeline. Zero days (default) preserves due-day notices.
ALTER TABLE family_care_items
 ADD COLUMN reminder_days_before INTEGER NOT NULL DEFAULT 0
 CHECK(reminder_days_before IN (0,1,2,3,7,14,30));

-- Advance notices are keyed to their target occurrence, not to the day they
-- are sent. Due notices and advance notices must never suppress each other.
ALTER TABLE notifications DROP CONSTRAINT IF EXISTS notifications_kind_check;
ALTER TABLE notifications ADD CONSTRAINT notifications_kind_check CHECK(kind IN (
 'event_assigned','event_family','bill_review',
 'event_confirmation','event_change_requested','event_confirmation_request',
 'event_schedule_changed','event_cancelled','care_due','care_advance'
));
CREATE UNIQUE INDEX IF NOT EXISTS notification_care_advance_occurrence_once
 ON notifications(user_id,care_item_id,kind,occurrence_start)
 WHERE kind='care_advance' AND care_item_id IS NOT NULL AND occurrence_start IS NOT NULL;

-- Do not opt existing customized accounts into a new delivery category.
-- They may enable advance notices separately in their own preferences.
ALTER TABLE member_notification_preferences
 DROP CONSTRAINT IF EXISTS member_notification_preferences_enabled_kinds_check;
ALTER TABLE member_notification_preferences
 ADD CONSTRAINT member_notification_preferences_enabled_kinds_check
 CHECK(enabled_kinds <@ ARRAY[
  'event_assigned','event_family','bill_review','care_due','care_advance'
 ]::TEXT[]);
ALTER TABLE member_notification_preferences
 ALTER COLUMN enabled_kinds SET DEFAULT ARRAY[
  'event_assigned','event_family','bill_review','care_due','care_advance'
 ]::TEXT[];
