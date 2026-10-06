CREATE TABLE reminder_deliveries (
    reminder_id UUID PRIMARY KEY REFERENCES reminders(id) ON DELETE CASCADE,
    status TEXT NOT NULL CHECK (status IN ('pending','sent','failed')),
    attempts INTEGER NOT NULL DEFAULT 0,
    last_error TEXT,
    sent_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
