-- Each family member controls whether others may request their appointment confirmation.
ALTER TABLE users ADD COLUMN confirmation_enabled BOOLEAN NOT NULL DEFAULT TRUE;
