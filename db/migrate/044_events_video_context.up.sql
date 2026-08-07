/* Events grow what-was-airing context plus a kind-specific payload, so a row
   can answer "what was on screen when this happened" without interval-pairing
   against video_plays. All three columns are NULL on kinds that predate them
   or have no airing context. video_ts_sec is seconds into the aired clip;
   writers that only know the clip (not the playhead) leave it NULL.
   state_crossing is the first kind to use meta: {"from","to","sequential"}. */
ALTER TABLE events
  ADD COLUMN video_id INTEGER,
  ADD COLUMN video_ts_sec REAL,
  ADD COLUMN meta JSONB;
