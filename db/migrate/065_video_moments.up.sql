/* The operator's marks on a moment of a clip, set from the console's video
   page: a starred moment worth coming back to, and a bad_coord where the
   clip's GPS track jumps or wanders off the road. Nothing in the bot's
   behaviour reads either yet; a bad_coord is there for the coords pass to
   take as an override.

   Keyed to the original recording like video_coords (source_ts_sec, the
   offset nothing re-cuts), so a re-trim moves no mark. */
CREATE TABLE video_moments (
  video_id      INTEGER NOT NULL REFERENCES videos (id),
  source_ts_sec REAL NOT NULL,
  kind          TEXT NOT NULL CHECK (kind IN ('star', 'bad_coord')),
  note          TEXT NOT NULL DEFAULT '',
  created_at    TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now(),
  PRIMARY KEY (video_id, kind, source_ts_sec)
);
