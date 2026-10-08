/* The insights API aggregates events by kind over a recent window
   (WHERE event = ... AND date_created >= ...). Both existing events indexes
   lead with username (011: username+date, 027: username+event+date), so a
   kind-filtered aggregate falls back to a full events scan. Leading with the
   kind keeps those reads proportional to the window instead of the table —
   the console's client gives these queries a 2-second budget.

   CONCURRENTLY so building it doesn't lock login/logout writes on the live
   events table. golang-migrate runs migrations without a transaction wrapper,
   so CONCURRENTLY is allowed — but this file must stay a SINGLE statement
   (a multi-statement file would be sent in one implicit transaction). */
CREATE INDEX CONCURRENTLY IF NOT EXISTS events_event_date
  ON events (event, date_created);
