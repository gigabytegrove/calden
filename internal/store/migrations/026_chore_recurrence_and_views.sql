-- Preserve existing chores and their status while adding household-wide participation.
ALTER TABLE chores ADD COLUMN recurrence_rule TEXT NOT NULL DEFAULT 'none'
    CHECK (recurrence_rule IN ('none','daily','weekly','biweekly','monthly','yearly'));
ALTER TABLE chores ADD COLUMN recurrence_until DATE;

CREATE TABLE chore_assignees (
    chore_id UUID NOT NULL REFERENCES chores(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id),
    PRIMARY KEY (chore_id, user_id)
);
INSERT INTO chore_assignees(chore_id,user_id)
    SELECT id,assigned_to FROM chores ON CONFLICT DO NOTHING;

CREATE TABLE chore_progress (
    chore_id UUID NOT NULL,
    user_id UUID NOT NULL,
    occurrence_date DATE NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending','in_progress','done')),
    completed_at TIMESTAMPTZ,
    completed_by UUID REFERENCES users(id) ON DELETE SET NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (chore_id,user_id,occurrence_date),
    FOREIGN KEY (chore_id,user_id) REFERENCES chore_assignees(chore_id,user_id) ON DELETE CASCADE
);
INSERT INTO chore_progress(chore_id,user_id,occurrence_date,status,completed_at,completed_by)
    SELECT id,assigned_to,due_date,status,completed_at,completed_by
    FROM chores WHERE status <> 'pending'
    ON CONFLICT DO NOTHING;
CREATE INDEX idx_chore_progress_date ON chore_progress(occurrence_date,status);

-- These are household-wide visibility preferences, not data-deletion controls.
INSERT INTO app_settings(key,value) VALUES
    ('bills_enabled','true'),('agenda_enabled','true')
ON CONFLICT(key) DO NOTHING;
