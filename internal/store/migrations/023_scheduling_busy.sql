-- Private per-user "busy" blocks. Never expose labels through conflict
-- previews, and do not permit users to read one another's blocks.
CREATE TABLE scheduling_busy_blocks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    starts_at TIMESTAMPTZ NOT NULL,
    ends_at TIMESTAMPTZ NOT NULL,
    label TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (ends_at>starts_at)
);
CREATE INDEX idx_scheduling_busy_user_range
    ON scheduling_busy_blocks(user_id,starts_at,ends_at);
