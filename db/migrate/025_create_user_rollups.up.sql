/* Derived per-(platform, username) aggregates, recomputed from the append-only
   events table by the rollup reconciler. events_miles is events-derived only
   (no subscriber bonus, no manual corrections) — users.miles stays the display
   number; this column is for audit and cross-platform aggregation.
   Keyed on (platform, username), not users.id: events rows key on it and it's
   the join key future identity linking will use. Bots are included here;
   readers filter is_bot via a users join. */
CREATE TABLE user_rollups (
  id            SERIAL PRIMARY KEY,
  platform      TEXT NOT NULL,
  username      VARCHAR(64) NOT NULL,
  events_miles  REAL NOT NULL DEFAULT 0.0,
  session_count INTEGER NOT NULL DEFAULT 0,
  first_seen    TIMESTAMP WITH TIME ZONE,
  last_seen     TIMESTAMP WITH TIME ZONE,
  date_updated  TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE (platform, username)
);

/* Reconciler progress, keyed on events.id (never date_created — historical
   rows carry zero-value dates). One row per rollup job. */
CREATE TABLE rollup_watermarks (
  name          TEXT PRIMARY KEY,
  last_event_id INTEGER NOT NULL DEFAULT 0,
  date_updated  TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO rollup_watermarks (name) VALUES ('user_rollups');
