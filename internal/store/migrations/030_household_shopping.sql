-- Family shopping lists, kept separate from calendars and preserved across feature changes.
CREATE TABLE household_shopping_lists (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name TEXT NOT NULL CHECK (char_length(btrim(name)) BETWEEN 1 AND 120),
  created_by UUID NOT NULL REFERENCES users(id),
  shared BOOLEAN NOT NULL DEFAULT TRUE,
  archived BOOLEAN NOT NULL DEFAULT FALSE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX shopping_lists_owner_idx ON household_shopping_lists(created_by,updated_at DESC);
CREATE TABLE household_shopping_items (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  list_id UUID NOT NULL REFERENCES household_shopping_lists(id) ON DELETE CASCADE,
  name TEXT NOT NULL CHECK (char_length(btrim(name)) BETWEEN 1 AND 200),
  quantity NUMERIC(12,3) NOT NULL DEFAULT 1 CHECK (quantity > 0 AND quantity <= 1000000),
  unit TEXT NOT NULL DEFAULT 'each' CHECK (unit IN ('each','pack','box','bag','lb','oz','kg','g','L','mL')),
  notes TEXT NOT NULL DEFAULT '',
  purchased BOOLEAN NOT NULL DEFAULT FALSE,
  purchased_at TIMESTAMPTZ,
  purchased_by UUID REFERENCES users(id) ON DELETE SET NULL,
  created_by UUID NOT NULL REFERENCES users(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX shopping_items_list_idx ON household_shopping_items(list_id,purchased,created_at);

-- Feature toggles are non-destructive. Existing lists are never deleted
-- when shopping is hidden from the household navigation.
INSERT INTO app_settings(key,value) VALUES('shopping_enabled','true')
ON CONFLICT(key) DO NOTHING;
