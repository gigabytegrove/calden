-- Central Tracking Hub (separate PostgreSQL database from Calden installs).
CREATE TABLE IF NOT EXISTS hub_installations (
  installation_id UUID PRIMARY KEY,
  token_hash CHAR(64) NOT NULL UNIQUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  enabled BOOLEAN NOT NULL DEFAULT true
);
CREATE TABLE IF NOT EXISTS hub_usage (
  subject TEXT NOT NULL,
  usage_day DATE NOT NULL DEFAULT CURRENT_DATE,
  request_count INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY(subject,usage_day)
);
CREATE TABLE IF NOT EXISTS hub_shipments (
  installation_id UUID NOT NULL REFERENCES hub_installations(installation_id) ON DELETE CASCADE,
  shipment_id UUID NOT NULL,
  tracking_number TEXT NOT NULL CHECK (length(tracking_number) BETWEEN 4 AND 100),
  carrier TEXT NOT NULL CHECK (carrier IN ('ups','usps','fedex','amazon','other')),
  provider_id TEXT NOT NULL DEFAULT '',
  provider_data JSONB,
  fetched_at TIMESTAMPTZ,
  next_fetch_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY(installation_id,shipment_id)
);
CREATE INDEX IF NOT EXISTS hub_shipments_updated_idx ON hub_shipments(updated_at);
