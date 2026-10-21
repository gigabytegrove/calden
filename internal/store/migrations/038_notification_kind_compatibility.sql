-- 038: safe forward correction for development databases which may have
-- applied the first care-notification migration before the original event
-- confirmation and event change kinds were carried forward.
ALTER TABLE notifications DROP CONSTRAINT IF EXISTS notifications_kind_check;
ALTER TABLE notifications ADD CONSTRAINT notifications_kind_check CHECK (kind IN (
 'event_assigned','event_family','bill_review','event_confirmation',
 'event_change_requested','event_confirmation_request','event_schedule_changed',
 'event_cancelled','care_due'
));
