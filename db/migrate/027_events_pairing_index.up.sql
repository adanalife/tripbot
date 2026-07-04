/* The rollup reconciler's pairing CTEs filter events by (username, event) and
   order by date_created; its first_seen/last_seen/session_count subqueries
   filter by (username, event) too. The existing events_username_date index
   (username, date_created) doesn't cover the event predicate, so the planner
   falls back to a full events scan on every tick — a ~280ms fixed floor per
   reconcile regardless of how few users are dirty (observed in prod 2026-07-04).
   This index turns those into per-user index scans. Platform stays a residual
   filter (near-constant, not selective enough to lead).

   CONCURRENTLY so building it doesn't lock login/logout writes on the live
   events table. golang-migrate runs migrations without a transaction wrapper,
   so CONCURRENTLY is allowed — but this file must stay a SINGLE statement
   (a multi-statement file would be sent in one implicit transaction). */
CREATE INDEX CONCURRENTLY IF NOT EXISTS events_username_event_date
  ON events (username, event, date_created);
