-- An independent, per-user bearer credential for opt-in mail gateway forwarding.
-- Only its SHA-256 digest is stored; the raw token is displayed once on creation.
CREATE TABLE package_inbound_tokens (
  owner_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  token_hash TEXT NOT NULL UNIQUE CHECK (char_length(token_hash)=64),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
