-- Coherent Monita retries across multiple Calden processes/replicas.
ALTER TABLE reminder_deliveries DROP CONSTRAINT IF EXISTS reminder_deliveries_status_check;
ALTER TABLE reminder_deliveries ADD CONSTRAINT reminder_deliveries_status_check
  CHECK (status IN ('pending','leased','sent','failed'));
ALTER TABLE reminder_deliveries ADD COLUMN next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now();
ALTER TABLE reminder_deliveries ADD COLUMN leased_until TIMESTAMPTZ;
CREATE INDEX idx_reminder_deliveries_due ON reminder_deliveries(next_attempt_at)
  WHERE status IN ('pending','failed','leased');
