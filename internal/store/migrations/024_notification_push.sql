CREATE TABLE notification_push_deliveries (
 notification_id UUID NOT NULL REFERENCES notifications(id) ON DELETE CASCADE,
 device_id UUID NOT NULL REFERENCES calden_devices(id) ON DELETE CASCADE,
 status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','leased','sent','failed')),
 attempts INTEGER NOT NULL DEFAULT 0,
 next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 leased_until TIMESTAMPTZ,
 sent_at TIMESTAMPTZ,
 last_error TEXT,
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 PRIMARY KEY(notification_id,device_id)
);
CREATE INDEX idx_notification_push_due ON notification_push_deliveries(next_attempt_at)
 WHERE status IN ('pending','leased');
CREATE OR REPLACE FUNCTION calden_enqueue_notification_push() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 INSERT INTO notification_push_deliveries(notification_id,device_id)
 SELECT NEW.id,d.id FROM calden_devices d WHERE d.user_id=NEW.user_id
 AND d.enabled AND d.revoked_at IS NULL AND d.push_provider='fcm' AND d.push_token IS NOT NULL
 ON CONFLICT(notification_id,device_id) DO UPDATE SET status='pending',attempts=0,
  next_attempt_at=now(),leased_until=NULL,sent_at=NULL,last_error=NULL,updated_at=now();
 RETURN NEW;
END $$;
CREATE TRIGGER calden_notify_push_created AFTER INSERT ON notifications
 FOR EACH ROW EXECUTE FUNCTION calden_enqueue_notification_push();
CREATE TRIGGER calden_notify_push_changed AFTER UPDATE OF title,message,created_at
 ON notifications FOR EACH ROW
 WHEN (OLD.title IS DISTINCT FROM NEW.title OR OLD.message IS DISTINCT FROM NEW.message
 OR OLD.created_at IS DISTINCT FROM NEW.created_at)
 EXECUTE FUNCTION calden_enqueue_notification_push();
