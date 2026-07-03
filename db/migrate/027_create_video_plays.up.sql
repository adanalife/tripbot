/* Append-only log of clip transitions on the live stream, written by the
   Twitch instance's video-change hook. Playback is random, so without this
   log "what was on screen at time T" is unreconstructible — it's the source
   of truth that lets achievement/state-visit state be audited or rebuilt,
   the same role events plays for miles. */
CREATE TABLE video_plays (
  id           SERIAL PRIMARY KEY,
  video_id     INTEGER NOT NULL REFERENCES videos(id),
  started_at   TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX video_plays_started_at ON video_plays (started_at);
