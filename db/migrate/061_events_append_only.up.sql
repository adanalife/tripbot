/* events is the append-only source of truth for session activity: users.miles
   and every rollup derive from it, and a wrong history is fixed by inserting a
   corrective event, never by editing or removing rows. These triggers make
   that a property of the table rather than a convention — UPDATE, DELETE and
   TRUNCATE raise, for the table owner too.

   A deliberate rewrite (a backfill migration, a forget-user request) states
   itself: ALTER TABLE events DISABLE TRIGGER events_append_only (and
   events_no_truncate), do the work, then ENABLE them again in the same
   migration. */
CREATE FUNCTION events_reject_mutation() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION 'events is append-only: % rejected', TG_OP
    USING HINT = 'Insert a corrective event instead.';
END;
$$;

CREATE TRIGGER events_append_only
  BEFORE UPDATE OR DELETE ON events
  FOR EACH ROW EXECUTE FUNCTION events_reject_mutation();

CREATE TRIGGER events_no_truncate
  BEFORE TRUNCATE ON events
  FOR EACH STATEMENT EXECUTE FUNCTION events_reject_mutation();
