-- Tasks & Chores is an independent household feature. Disabled by default.
INSERT INTO app_settings(key,value) VALUES('chores_enabled','false')
ON CONFLICT(key) DO NOTHING;

CREATE TABLE chores(
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 title TEXT NOT NULL CHECK (length(btrim(title))>0),
 notes TEXT NOT NULL DEFAULT '',
 due_date DATE NOT NULL,
 assigned_to UUID NOT NULL REFERENCES users(id),
 created_by UUID NOT NULL REFERENCES users(id),
 status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','in_progress','done')),
 completed_at TIMESTAMPTZ,
 completed_by UUID REFERENCES users(id) ON DELETE SET NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_chores_due ON chores(status,due_date);
CREATE INDEX idx_chores_assignee ON chores(assigned_to,due_date);
