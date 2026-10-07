ALTER TABLE event_recurrence
    ADD COLUMN raw_rule TEXT NOT NULL DEFAULT '';

CREATE TABLE calendar_import_sources (
    provider TEXT NOT NULL,
    external_id TEXT NOT NULL,
    calendar_id UUID NOT NULL REFERENCES calendars(id) ON DELETE CASCADE,
    display_name TEXT NOT NULL DEFAULT '',
    last_imported_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (provider, external_id)
);

CREATE INDEX idx_calendar_import_sources_calendar
    ON calendar_import_sources(calendar_id);
