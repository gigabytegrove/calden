CREATE TABLE packages (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 owner_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 title VARCHAR(160) NOT NULL,
 carrier VARCHAR(16) NOT NULL CHECK (carrier IN ('amazon','usps','ups','fedex','other')),
 tracking_number VARCHAR(120) NOT NULL DEFAULT '',
 expected_date DATE,
 status VARCHAR(24) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','label_created','in_transit','out_for_delivery','delivered','delayed','exception')),
 shared BOOLEAN NOT NULL DEFAULT FALSE,
 notes VARCHAR(2000) NOT NULL DEFAULT '',
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX packages_owner_idx ON packages(owner_id);
CREATE INDEX packages_delivery_idx ON packages(expected_date) WHERE expected_date IS NOT NULL;
