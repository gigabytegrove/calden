-- Family routines: recurring daily checklists with stable step identities.
-- Archiving retains completed-instance history. This migration is additive.
CREATE TABLE family_routines (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 title TEXT NOT NULL CHECK (char_length(btrim(title)) BETWEEN 1 AND 120),
 notes TEXT NOT NULL DEFAULT '',
 assigned_to UUID NOT NULL REFERENCES users(id),
 created_by UUID NOT NULL REFERENCES users(id),
 time_of_day TEXT NOT NULL DEFAULT 'anytime'
   CHECK (time_of_day IN ('morning','evening','anytime')),
 days_of_week SMALLINT[] NOT NULL DEFAULT ARRAY[1,2,3,4,5,6,7]::SMALLINT[]
   CHECK (cardinality(days_of_week) BETWEEN 1 AND 7
     AND days_of_week <@ ARRAY[1,2,3,4,5,6,7]::SMALLINT[]),
 steps JSONB NOT NULL DEFAULT '[]'::jsonb
   CHECK (jsonb_typeof(steps)='array'),
 archived BOOLEAN NOT NULL DEFAULT FALSE,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_family_routines_person ON family_routines(assigned_to,archived);
CREATE TABLE family_routine_progress (
 routine_id UUID NOT NULL REFERENCES family_routines(id),
 user_id UUID NOT NULL REFERENCES users(id),
 step_id UUID NOT NULL,
 occurrence_date DATE NOT NULL,
 step_title_snapshot TEXT NOT NULL,
 completed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 completed_by UUID NOT NULL REFERENCES users(id),
 PRIMARY KEY(routine_id,user_id,step_id,occurrence_date)
);
CREATE INDEX idx_routine_progress_person_date ON family_routine_progress(user_id,occurrence_date DESC);
INSERT INTO app_settings(key,value) VALUES('routines_enabled','false')
ON CONFLICT(key) DO NOTHING;
