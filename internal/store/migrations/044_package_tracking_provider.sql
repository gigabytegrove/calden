-- Authorized carrier API lookups are opt-in and independent from email import.
CREATE TABLE package_tracking_provider (
 id SMALLINT PRIMARY KEY DEFAULT 1 CHECK (id=1),
 sealed_api_key TEXT NOT NULL CHECK (length(sealed_api_key)>20),
 enabled BOOLEAN NOT NULL DEFAULT true,
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
ALTER TABLE package_shipments
 ADD COLUMN provider_tracking_id TEXT NOT NULL DEFAULT '',
 ADD COLUMN provider_slug TEXT NOT NULL DEFAULT '',
 ADD COLUMN last_tracking_check_at TIMESTAMPTZ,
 ADD COLUMN next_tracking_check_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 ADD COLUMN tracking_error TEXT NOT NULL DEFAULT '';
CREATE INDEX package_provider_due_idx ON package_shipments(next_tracking_check_at)
 WHERE NOT archived AND status!='delivered';
ALTER TABLE package_checkpoints ADD COLUMN provider_ref TEXT NOT NULL DEFAULT '';
CREATE UNIQUE INDEX package_provider_checkpoint_uq
 ON package_checkpoints(shipment_id,provider_ref) WHERE provider_ref<>'';
