CREATE TABLE bill_allocations (
    event_id UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    occurrence_start TIMESTAMPTZ NOT NULL,
    amount_allocated NUMERIC(12,2) NOT NULL,
    allocated_on DATE NOT NULL DEFAULT CURRENT_DATE,
    allocated_by_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    allocated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (event_id, occurrence_start),
    CHECK (amount_allocated > 0)
);

CREATE INDEX idx_bill_allocations_allocated_by
    ON bill_allocations(allocated_by_user_id);

ALTER TABLE notifications
    DROP CONSTRAINT IF EXISTS notifications_kind_check;

ALTER TABLE notifications
    ADD CONSTRAINT notifications_kind_check
    CHECK (kind IN ('event_assigned','event_family','bill_review'));

ALTER TABLE notifications
    ADD COLUMN occurrence_start TIMESTAMPTZ;

CREATE UNIQUE INDEX idx_notifications_bill_review_occurrence
    ON notifications(user_id,event_id,kind,occurrence_start)
    WHERE kind='bill_review' AND occurrence_start IS NOT NULL;
