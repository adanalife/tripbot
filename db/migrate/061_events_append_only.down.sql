DROP TRIGGER IF EXISTS events_no_truncate ON events;
DROP TRIGGER IF EXISTS events_append_only ON events;
DROP FUNCTION IF EXISTS events_reject_mutation();
