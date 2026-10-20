-- 032: Family rewards/allowances; additive to existing recurring chore data.
ALTER TABLE chores ADD COLUMN IF NOT EXISTS reward_points INTEGER NOT NULL DEFAULT 0
  CHECK (reward_points BETWEEN 0 AND 10000);

CREATE TABLE family_chore_reviews(
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  chore_id UUID NOT NULL REFERENCES chores(id) ON DELETE RESTRICT,
  user_id UUID NOT NULL REFERENCES users(id),
  occurrence_date DATE NOT NULL,
  decision TEXT NOT NULL CHECK (decision IN ('approved','rejected')),
  points_awarded INTEGER NOT NULL DEFAULT 0 CHECK (points_awarded BETWEEN 0 AND 10000),
  reviewed_by UUID NOT NULL REFERENCES users(id),
  note TEXT NOT NULL DEFAULT '',
  reviewed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(chore_id,user_id,occurrence_date)
);
CREATE INDEX family_chore_reviews_user_idx ON family_chore_reviews(user_id,reviewed_at DESC);

CREATE TABLE family_point_ledger(
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id),
  delta INTEGER NOT NULL CHECK (delta<>0 AND delta BETWEEN -100000 AND 100000),
  kind TEXT NOT NULL CHECK(kind IN ('chore','adjustment','redemption','refund')),
  source_id UUID,
  description TEXT NOT NULL,
  created_by UUID NOT NULL REFERENCES users(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX family_point_unique_source ON family_point_ledger(kind,source_id)
  WHERE source_id IS NOT NULL;
CREATE INDEX family_point_by_user ON family_point_ledger(user_id,created_at DESC);

CREATE TABLE family_reward_catalog(
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  title TEXT NOT NULL CHECK(char_length(btrim(title)) BETWEEN 1 AND 140),
  description TEXT NOT NULL DEFAULT '',
  cost_points INTEGER NOT NULL CHECK(cost_points BETWEEN 1 AND 100000),
  active BOOLEAN NOT NULL DEFAULT TRUE,
  created_by UUID NOT NULL REFERENCES users(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE family_reward_claims(
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id),
  reward_id UUID NOT NULL REFERENCES family_reward_catalog(id),
  title_snapshot TEXT NOT NULL,
  cost_points INTEGER NOT NULL CHECK(cost_points>0),
  status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','fulfilled','cancelled')),
  request_id UUID NOT NULL UNIQUE,
  reviewed_by UUID REFERENCES users(id),
  reviewed_at TIMESTAMPTZ,
  requested_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX family_claims_user_idx ON family_reward_claims(user_id,requested_at DESC);

CREATE TABLE family_allowance_plans(
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL UNIQUE REFERENCES users(id),
  amount_cents BIGINT NOT NULL CHECK(amount_cents BETWEEN 1 AND 100000000),
  frequency TEXT NOT NULL CHECK(frequency IN ('weekly','monthly')),
  anchor_day SMALLINT NOT NULL CHECK(anchor_day BETWEEN 1 AND 31),
  next_due_date DATE NOT NULL,
  active BOOLEAN NOT NULL DEFAULT TRUE,
  created_by UUID NOT NULL REFERENCES users(id),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE family_allowance_entries(
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id),
  plan_id UUID REFERENCES family_allowance_plans(id),
  due_date DATE,
  kind TEXT NOT NULL CHECK(kind IN ('due','paid')),
  amount_cents BIGINT NOT NULL CHECK(amount_cents BETWEEN 1 AND 100000000),
  note TEXT NOT NULL DEFAULT '',
  request_id UUID UNIQUE,
  recorded_by UUID NOT NULL REFERENCES users(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK ((kind='due' AND plan_id IS NOT NULL AND due_date IS NOT NULL) OR
         (kind='paid' AND plan_id IS NULL AND due_date IS NULL AND request_id IS NOT NULL))
);
CREATE UNIQUE INDEX allowance_due_unique ON family_allowance_entries(plan_id,due_date) WHERE kind='due';
CREATE INDEX allowance_entry_user_idx ON family_allowance_entries(user_id,created_at DESC);

INSERT INTO app_settings(key,value) VALUES ('rewards_enabled','false')
 ON CONFLICT(key) DO NOTHING;
