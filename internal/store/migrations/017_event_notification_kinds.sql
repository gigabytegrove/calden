-- Extend notification taxonomy without altering existing notification records.
ALTER TABLE notifications DROP CONSTRAINT IF EXISTS notifications_kind_check;
ALTER TABLE notifications ADD CONSTRAINT notifications_kind_check CHECK (kind IN (
  'event_assigned', 'event_family', 'bill_review',
  'event_confirmation', 'event_change_requested',
  'event_confirmation_request', 'event_schedule_changed',
  'event_cancelled'
));
