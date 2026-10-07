ALTER TABLE bill_payments
    DROP CONSTRAINT bill_payments_pkey;

ALTER TABLE bill_payments
    ADD COLUMN id UUID DEFAULT gen_random_uuid();

ALTER TABLE bill_payments
    ALTER COLUMN id SET NOT NULL;

ALTER TABLE bill_payments
    ADD CONSTRAINT bill_payments_pkey PRIMARY KEY (id);

ALTER TABLE bill_payments
    ADD COLUMN paid_on DATE,
    ADD COLUMN cleared_on DATE,
    ADD COLUMN settles_bill BOOLEAN NOT NULL DEFAULT TRUE;

UPDATE bill_payments
SET paid_on = COALESCE(paid_on, paid_at::date);

ALTER TABLE bill_payments
    ALTER COLUMN paid_on SET NOT NULL,
    ALTER COLUMN paid_on SET DEFAULT CURRENT_DATE;

CREATE INDEX idx_bill_payments_occurrence
    ON bill_payments(event_id, occurrence_start);

CREATE TABLE bill_no_balance_occurrences (
    event_id UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    occurrence_start TIMESTAMPTZ NOT NULL,
    marked_by_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    marked_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (event_id, occurrence_start)
);
