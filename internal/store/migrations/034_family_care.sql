-- 034: school, household equipment and pet care. Additive; all modules disabled
-- until the household explicitly enables them. Historical completions survive archive.
CREATE TABLE family_care_subjects(
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 kind TEXT NOT NULL CHECK (kind IN ('school','maintenance','pets')),
 name TEXT NOT NULL CHECK(char_length(btrim(name)) BETWEEN 1 AND 140),
 details TEXT NOT NULL DEFAULT '',
 child_user_id UUID REFERENCES users(id),
 school_name TEXT NOT NULL DEFAULT '',
 grade TEXT NOT NULL DEFAULT '',
 archived BOOLEAN NOT NULL DEFAULT FALSE,
 created_by UUID NOT NULL REFERENCES users(id),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 CHECK ((kind='school') OR (child_user_id IS NULL AND school_name='' AND grade=''))
);
CREATE INDEX family_care_subject_kind ON family_care_subjects(kind,archived);
CREATE INDEX family_care_subject_child ON family_care_subjects(child_user_id) WHERE child_user_id IS NOT NULL;

CREATE TABLE family_school_terms(
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 subject_id UUID NOT NULL REFERENCES family_care_subjects(id),
 title TEXT NOT NULL CHECK(char_length(btrim(title)) BETWEEN 1 AND 120),
 begins_on DATE NOT NULL,
 ends_on DATE NOT NULL,
 created_by UUID NOT NULL REFERENCES users(id),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 CHECK(ends_on>=begins_on),
 UNIQUE(subject_id,title,begins_on)
);
CREATE INDEX family_school_terms_subject ON family_school_terms(subject_id,begins_on);

CREATE TABLE family_care_items(
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 subject_id UUID NOT NULL REFERENCES family_care_subjects(id),
 kind TEXT NOT NULL CHECK(kind IN ('event','deadline','closure','service','check','appointment','vaccination','medication')),
 title TEXT NOT NULL CHECK(char_length(btrim(title)) BETWEEN 1 AND 180),
 details TEXT NOT NULL DEFAULT '',
 place TEXT NOT NULL DEFAULT '',
 contact TEXT NOT NULL DEFAULT '',
 starts_on DATE NOT NULL,
 repeat_rule TEXT NOT NULL DEFAULT 'none' CHECK(repeat_rule IN ('none','daily','weekly','monthly','yearly')),
 term_id UUID REFERENCES family_school_terms(id),
 assigned_to UUID REFERENCES users(id),
 archived BOOLEAN NOT NULL DEFAULT FALSE,
 created_by UUID NOT NULL REFERENCES users(id),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX family_care_items_subject ON family_care_items(subject_id,archived,starts_on);

CREATE TABLE family_care_occurrences(
 item_id UUID NOT NULL REFERENCES family_care_items(id),
 occurrence_date DATE NOT NULL,
 status TEXT NOT NULL CHECK(status IN ('completed','skipped')),
 note TEXT NOT NULL DEFAULT '',
 recorded_by UUID NOT NULL REFERENCES users(id),
 recorded_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 PRIMARY KEY(item_id,occurrence_date)
);
CREATE INDEX family_care_occurrences_date ON family_care_occurrences(occurrence_date,recorded_at);

INSERT INTO app_settings(key,value) VALUES
 ('school_enabled','false'),('maintenance_enabled','false'),('pets_enabled','false')
 ON CONFLICT(key) DO NOTHING;
