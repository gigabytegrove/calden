ALTER TABLE notifications ADD COLUMN dismissed_at TIMESTAMPTZ;
CREATE INDEX idx_notifications_visible_user_created ON notifications(user_id, created_at DESC) WHERE dismissed_at IS NULL;
ALTER TABLE reminders ADD COLUMN recipient_user_id UUID REFERENCES users(id) ON DELETE SET NULL;
CREATE INDEX idx_reminders_recipient ON reminders(recipient_user_id) WHERE recipient_user_id IS NOT NULL;
