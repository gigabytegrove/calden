CREATE TABLE bill_payments (
    event_id UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    occurrence_start TIMESTAMPTZ NOT NULL,
    paid_by_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    amount_paid NUMERIC(12,2),
    paid_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (event_id, occurrence_start),
    CHECK (amount_paid IS NULL OR amount_paid >= 0)
);

CREATE INDEX idx_bill_payments_paid_by
    ON bill_payments(paid_by_user_id);

CREATE INDEX idx_bill_payments_paid_at
    ON bill_payments(paid_at);
