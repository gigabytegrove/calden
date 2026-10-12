-- Bump a durable revision on every meaningful household calendar mutation.
-- The notification WebSocket streams a version marker without exposing event
-- contents. Clients re-fetch only data their bearer token permits.
INSERT INTO app_settings(key,value) VALUES('data_revision', gen_random_uuid()::text)
ON CONFLICT(key) DO NOTHING;

CREATE OR REPLACE FUNCTION calden_bump_data_revision() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  UPDATE app_settings SET value=gen_random_uuid()::text, updated_at=now()
  WHERE key='data_revision';
  RETURN NULL;
END;
$$;

DO $$
DECLARE t text;
BEGIN
  FOREACH t IN ARRAY ARRAY[
    'events','event_assignees','event_recurrence','event_occurrence_exceptions',
    'calendars','calendar_permissions','categories','reminders',
    'bill_event_details','bill_payments','bill_allocations','bill_no_balance_occurrences',
    'chores'
  ] LOOP
    IF to_regclass('public.' || t) IS NOT NULL THEN
      EXECUTE format('CREATE TRIGGER %I AFTER INSERT OR UPDATE OR DELETE ON %I
        FOR EACH STATEMENT EXECUTE FUNCTION calden_bump_data_revision()', 'calden_sync_'||t, t);
    END IF;
  END LOOP;
END $$;
