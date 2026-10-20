-- Personal dashboard settings are additive and private to their owner.
CREATE TABLE IF NOT EXISTS member_dashboard_preferences (
  user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  appearance TEXT NOT NULL DEFAULT 'system'
    CHECK (appearance IN ('system','light','dark')),
  layout TEXT NOT NULL DEFAULT 'standard'
    CHECK (layout IN ('standard','simple')),
  widgets JSONB NOT NULL DEFAULT '["calendar","notifications"]'::jsonb
    CHECK (jsonb_typeof(widgets) = 'array'),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
