-- Approved family customization: only presentation labels, icons and theme.
-- Canonical module IDs and permissions never depend on these values.
CREATE TABLE family_module_presentation(
 module_id TEXT PRIMARY KEY,
 display_label TEXT NOT NULL CHECK(char_length(btrim(display_label)) BETWEEN 1 AND 32),
 icon TEXT NOT NULL DEFAULT 'grid'
  CHECK(icon IN ('grid','calendar','house','shopping-cart','list-check','gift',
      'graduation-cap','wrench','paw-print','bell','wallet','users','utensils','heart','clipboard-list')),
 updated_by UUID NOT NULL REFERENCES users(id),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
INSERT INTO app_settings(key,value) VALUES('family_theme_preset','classic')
 ON CONFLICT(key) DO NOTHING;
