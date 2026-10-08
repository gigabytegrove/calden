-- Additive foundation for per-user devices and per-occurrence confirmations.
CREATE TABLE calden_devices (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    installation_id UUID NOT NULL,
    name TEXT NOT NULL,
    platform TEXT NOT NULL CHECK (platform IN ('android','web','ios')),
    push_provider TEXT NOT NULL DEFAULT 'none' CHECK (push_provider IN ('none','fcm','webpush')),
    push_token TEXT,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at TIMESTAMPTZ,
    UNIQUE(user_id, installation_id)
);
CREATE INDEX calden_devices_active_user ON calden_devices(user_id) WHERE revoked_at IS NULL;

ALTER TABLE events ADD COLUMN request_confirmation BOOLEAN NOT NULL DEFAULT FALSE;
CREATE TABLE event_confirmations (
    event_id UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    occurrence_start TIMESTAMPTZ NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','confirmed','change_requested')),
    reason TEXT NOT NULL DEFAULT '',
    responded_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (event_id, user_id, occurrence_start)
);
CREATE INDEX event_confirmations_user_status ON event_confirmations(user_id,status);

-- Allow the two confirmation response notification types.
ALTER TABLE notifications DROP CONSTRAINT IF EXISTS notifications_kind_check;
ALTER TABLE notifications ADD CONSTRAINT notifications_kind_check
  CHECK (kind IN ('event_assigned','event_family','bill_review','event_confirmation','event_change_requested'));
