-- Optional per-adult read-only IMAP discovery.
-- The encrypted app password is never sent back to a browser.
CREATE TABLE package_mailboxes (
 owner_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
 hostname TEXT NOT NULL CHECK (length(hostname) BETWEEN 4 AND 253),
 username TEXT NOT NULL CHECK (length(username) BETWEEN 3 AND 254),
 encrypted_password TEXT NOT NULL,
 enabled BOOLEAN NOT NULL DEFAULT FALSE,
 uid_validity BIGINT NOT NULL DEFAULT 0 CHECK(uid_validity BETWEEN 0 AND 4294967295),
 last_uid BIGINT NOT NULL DEFAULT 0 CHECK(last_uid BETWEEN 0 AND 4294967295),
 next_scan TIMESTAMPTZ NOT NULL DEFAULT now(),
 last_scan TIMESTAMPTZ,
 last_error TEXT NOT NULL DEFAULT '',
 last_imported INTEGER NOT NULL DEFAULT 0 CHECK(last_imported>=0),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX package_mailbox_next_scan_idx ON package_mailboxes(next_scan) WHERE enabled;
