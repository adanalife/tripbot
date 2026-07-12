/* Append-only raw signals for footage-performance analytics, mirroring the
   eventbus emissions into durable storage (NATS core is fire-and-forget).
   video_plays: one row per clip switch — the permanent timeline of what was
   on screen when. viewer_samples: one row per ~61s viewer-count tick.
   state/flagged/lat/lng are denormalized at play time because videos rows
   mutate afterwards (coord backfills, state interpolation); the play row
   records what was true on screen. */
CREATE TABLE video_plays (
  id           SERIAL PRIMARY KEY,
  platform     TEXT NOT NULL,
  video_id     INTEGER,
  state        TEXT NOT NULL DEFAULT '',
  flagged      BOOLEAN NOT NULL DEFAULT false,
  lat          DOUBLE PRECISION NOT NULL DEFAULT 0,
  lng          DOUBLE PRECISION NOT NULL DEFAULT 0,
  started_at   TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX video_plays_platform_started_at ON video_plays (platform, started_at);

CREATE TABLE viewer_samples (
  id           SERIAL PRIMARY KEY,
  platform     TEXT NOT NULL,
  count        INTEGER NOT NULL,
  video_id     INTEGER,
  sampled_at   TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX viewer_samples_platform_sampled_at ON viewer_samples (platform, sampled_at);
