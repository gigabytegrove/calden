-- Native, optional household package tracking. Never create calendar events for shipments.
INSERT INTO app_settings(key,value) VALUES('packages_enabled','false') ON CONFLICT(key) DO NOTHING;

CREATE TABLE package_shipments (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  shared BOOLEAN NOT NULL DEFAULT false,
  label TEXT NOT NULL CHECK (char_length(label) BETWEEN 1 AND 140),
  carrier TEXT NOT NULL CHECK (carrier IN ('amazon','usps','ups','fedex','other')),
  tracking_number TEXT NOT NULL CHECK (char_length(tracking_number) BETWEEN 4 AND 100),
  status TEXT NOT NULL DEFAULT 'unknown' CHECK (status IN ('unknown','label_created','in_transit','out_for_delivery','delivered','exception')),
  status_source TEXT NOT NULL DEFAULT 'manual' CHECK (status_source IN ('manual','email','provider')),
  expected_date DATE,
  delivered_at TIMESTAMPTZ,
  notes TEXT NOT NULL DEFAULT '',
  archived BOOLEAN NOT NULL DEFAULT false,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX package_owner_carrier_tracking_uq ON package_shipments(owner_id,carrier,upper(tracking_number));
CREATE INDEX package_visible_dates_idx ON package_shipments(expected_date) WHERE archived=false;
CREATE INDEX package_owner_updated_idx ON package_shipments(owner_id,updated_at DESC);

CREATE TABLE package_checkpoints (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  shipment_id UUID NOT NULL REFERENCES package_shipments(id) ON DELETE CASCADE,
  actor_id UUID REFERENCES users(id) ON DELETE SET NULL,
  status TEXT NOT NULL,
  detail TEXT NOT NULL DEFAULT '',
  source TEXT NOT NULL CHECK (source IN ('manual','email','provider')),
  occurred_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX package_checkpoints_ship_idx ON package_checkpoints(shipment_id,occurred_at DESC,id DESC);
