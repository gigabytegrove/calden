-- Personal notification delivery preferences. Existing persistent inbox is unchanged.
CREATE TABLE member_notification_preferences(
 user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
 push_enabled BOOLEAN NOT NULL DEFAULT TRUE,
 live_enabled BOOLEAN NOT NULL DEFAULT TRUE,
 enabled_kinds TEXT[] NOT NULL DEFAULT ARRAY['event_assigned','event_family','bill_review']::TEXT[]
  CHECK (enabled_kinds <@ ARRAY['event_assigned','event_family','bill_review']::TEXT[]),
 quiet_start TEXT,
 quiet_end TEXT,
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 CHECK ((quiet_start IS NULL AND quiet_end IS NULL) OR
        (quiet_start ~ '^([01][0-9]|2[0-3]):[0-5][0-9]$'
         AND quiet_end ~ '^([01][0-9]|2[0-3]):[0-5][0-9]$'
         AND quiet_start<>quiet_end))
);
