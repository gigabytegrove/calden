ALTER TABLE events
    ADD COLUMN recurrence_parent_id UUID REFERENCES events(id) ON DELETE CASCADE,
    ADD COLUMN recurrence_original_start TIMESTAMPTZ;

CREATE UNIQUE INDEX idx_events_recurrence_override
    ON events(recurrence_parent_id, recurrence_original_start)
    WHERE recurrence_parent_id IS NOT NULL AND recurrence_original_start IS NOT NULL;

CREATE TABLE event_occurrence_exceptions (
    event_id UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    original_start TIMESTAMPTZ NOT NULL,
    cancelled BOOLEAN NOT NULL DEFAULT false,
    replacement_event_id UUID REFERENCES events(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (event_id, original_start),
    CHECK (cancelled OR replacement_event_id IS NOT NULL)
);

CREATE INDEX idx_occurrence_exceptions_replacement
    ON event_occurrence_exceptions(replacement_event_id)
    WHERE replacement_event_id IS NOT NULL;
