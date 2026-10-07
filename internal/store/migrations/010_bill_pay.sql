ALTER TABLE calendars
    ADD COLUMN calendar_type TEXT NOT NULL DEFAULT 'standard'
    CHECK (calendar_type IN ('standard','bill_pay'));

UPDATE calendars
SET calendar_type='bill_pay', updated_at=now()
WHERE lower(trim(name)) IN ('bills','bill pay','bill payment','bill payments')
   OR (icon='receipt' AND lower(description) LIKE '%bill%');

CREATE TABLE bill_event_details (
    event_id UUID PRIMARY KEY REFERENCES events(id) ON DELETE CASCADE,
    amount_due NUMERIC(12,2),
    amount_is_estimate BOOLEAN NOT NULL DEFAULT FALSE,
    payer_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (amount_due IS NULL OR amount_due >= 0)
);

CREATE INDEX idx_bill_event_details_payer
    ON bill_event_details(payer_user_id);
