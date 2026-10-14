-- Keep existing entries as chores. Tasks are a distinct, optional kind.
-- Disabling either feature never removes assignments or completion history.
ALTER TABLE chores ADD COLUMN IF NOT EXISTS kind TEXT NOT NULL DEFAULT 'chore';
ALTER TABLE chores DROP CONSTRAINT IF EXISTS chores_kind_allowed;
ALTER TABLE chores ADD CONSTRAINT chores_kind_allowed CHECK (kind IN ('task','chore'));
CREATE INDEX IF NOT EXISTS chores_kind_due_idx ON chores(kind,due_date);
