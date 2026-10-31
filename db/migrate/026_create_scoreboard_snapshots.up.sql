/* Immutable month-end snapshots of the monthly scoreboards (miles_YYYY_MM,
   guess_state_YYYY_MM), written once per board by the rollup reconciler after
   rollover. Monthly boards themselves stay mutable-in-place and roll over
   silently; this table is what lets a future !lastmonth render history. */
CREATE TABLE scoreboard_snapshots (
  id              SERIAL PRIMARY KEY,
  scoreboard_name VARCHAR(64) NOT NULL,
  platform        TEXT NOT NULL,
  rank            INTEGER NOT NULL,
  username        VARCHAR(64) NOT NULL,
  value           REAL NOT NULL,
  date_created    TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE (scoreboard_name, platform, rank)
);
