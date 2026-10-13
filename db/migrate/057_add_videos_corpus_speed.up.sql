/* Which dashcam trip a clip comes from, and how fast its footage runs.
   corpus partitions the table: s1 is the 2018 real-time footage, s2 the
   2026 real-time footage, s2fast the 2026 footage the camera recorded in its
   6x time-lapse mode. Two s2 pieces with identical names can differ only by
   speed, so the column carries it rather than the slug: speed is how many
   real seconds pass per second of clip (1, or 6 for s2fast). Anything that
   differences over clip time — derived velocity, elapsed-time readers —
   multiplies by it. Written by the pipeline's ingest stage; every row that
   exists before this migration is season 1 at real time. */
ALTER TABLE videos
  ADD COLUMN corpus TEXT NOT NULL DEFAULT 's1'
    CHECK (corpus IN ('s1', 's2', 's2fast')),
  ADD COLUMN speed SMALLINT NOT NULL DEFAULT 1
    CHECK (speed > 0);
