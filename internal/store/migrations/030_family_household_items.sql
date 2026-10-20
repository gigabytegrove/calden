-- Family-platform household records. Every entry is either shared or belongs to one person.
-- Disabling features never deletes records or completion history.
CREATE TABLE household_items (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 section TEXT NOT NULL CHECK(section IN ('shopping','school','maintenance','pets','routines')),
 title TEXT NOT NULL CHECK(length(btrim(title)) BETWEEN 1 AND 160),
 notes TEXT NOT NULL DEFAULT '',
 subject TEXT NOT NULL DEFAULT '',
 list_name TEXT NOT NULL DEFAULT '',
 quantity NUMERIC(12,3) NOT NULL DEFAULT 1 CHECK(quantity>0 AND quantity<=1000000),
 unit TEXT NOT NULL DEFAULT '',
 due_on DATE,
 recurrence TEXT NOT NULL DEFAULT 'none' CHECK(recurrence IN ('none','daily','weekly','biweekly','monthly','yearly')),
 private BOOLEAN NOT NULL DEFAULT FALSE,
 owner_id UUID REFERENCES users(id) ON DELETE CASCADE,
 created_by UUID NOT NULL REFERENCES users(id),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 CHECK (private=(owner_id IS NOT NULL)),
 CHECK (recurrence='none' OR due_on IS NOT NULL)
);
CREATE INDEX household_items_section_access ON household_items(section,owner_id,due_on);
CREATE TABLE household_item_completions (
 item_id UUID NOT NULL REFERENCES household_items(id) ON DELETE CASCADE,
 occurrence_date DATE NOT NULL,
 completed_by UUID NOT NULL REFERENCES users(id),
 completed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 PRIMARY KEY(item_id,occurrence_date)
);
CREATE INDEX household_completions_user ON household_item_completions(completed_by,completed_at);
INSERT INTO app_settings(key,value) VALUES
('family_shopping_enabled','false'),
('family_school_enabled','false'),
('family_maintenance_enabled','false'),
('family_pets_enabled','false'),
('family_routines_enabled','false')
ON CONFLICT(key) DO NOTHING;
