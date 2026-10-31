/* The day's first look at each live monthly board (miles_YYYY_MM,
   guess_state_YYYY_MM), top 50 per platform, written once per day by the
   rollup tick so the leaderboards can show how far a viewer has moved since the
   start of the day. Kept apart from scoreboard_snapshots, whose rows are the
   frozen months the leaderboard month selector lists. The same tick prunes
   rows older than 40 days. */
CREATE TABLE scoreboard_daily_snapshots (
  day             DATE NOT NULL,
  scoreboard_name VARCHAR(64) NOT NULL,
  platform        TEXT NOT NULL,
  rank            INTEGER NOT NULL,
  username        VARCHAR(64) NOT NULL,
  value           REAL NOT NULL,
  date_created    TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (day, scoreboard_name, platform, username)
);
