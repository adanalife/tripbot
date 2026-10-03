/* One row per (platform, username) day on which the viewer caught footage
   from a state — the substrate for the tiered state-visit achievements
   ("10th visit to California" = 10 distinct days with California footage on
   screen). Day granularity keeps a 3-hour binge = one visit. Rebuildable
   from video_plays × events if ever lost. */
CREATE TABLE user_state_days (
  platform     TEXT NOT NULL,
  username     VARCHAR(64) NOT NULL,
  state        VARCHAR(50) NOT NULL,
  day          DATE NOT NULL,
  PRIMARY KEY (platform, username, state, day)
);

/* Earned achievements. name is the stable key ("state-california-10",
   "landmark-old-faithful"); title is the display string chat sees. The
   UNIQUE constraint is what makes awarding idempotent — awards are
   INSERT ... ON CONFLICT DO NOTHING, announce only on actual insert. */
CREATE TABLE achievements (
  id           SERIAL PRIMARY KEY,
  platform     TEXT NOT NULL,
  username     VARCHAR(64) NOT NULL,
  name         TEXT NOT NULL,
  title        TEXT NOT NULL,
  earned_at    TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE (platform, username, name)
);
