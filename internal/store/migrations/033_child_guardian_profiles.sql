-- 033: Guarded child accounts and designated guardian relationships.
-- Existing accounts remain adult by default; no existing role is changed.
ALTER TABLE users ADD COLUMN is_child BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE users ADD CONSTRAINT child_account_restricted_role
  CHECK (NOT is_child OR role='restricted');

CREATE TABLE family_child_profiles (
  user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  display_mode TEXT NOT NULL DEFAULT 'simple'
    CHECK (display_mode IN ('simple','standard')),
  allowed_modules TEXT[] NOT NULL DEFAULT ARRAY['calendar','chores','routines','rewards']::TEXT[]
    CHECK (cardinality(allowed_modules) BETWEEN 1 AND 8),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE family_child_guardians (
  child_id UUID NOT NULL REFERENCES family_child_profiles(user_id) ON DELETE CASCADE,
  guardian_id UUID NOT NULL REFERENCES users(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (child_id,guardian_id),
  CHECK(child_id<>guardian_id)
);
CREATE INDEX family_child_guardians_guardian ON family_child_guardians(guardian_id,child_id);
