/* A report on a whole clip (!boring: "this one's dull") points at no second,
   so source_ts_sec goes nullable and such a report is one row per
   (video_id, kind, username, platform). Postgres refuses a nullable column in
   a primary key, so the key becomes a unique index over
   COALESCE(source_ts_sec, -1), a second no recording has. Every existing row
   keeps its second and its place in the key. */
ALTER TABLE video_moments
  DROP CONSTRAINT video_moments_pkey,
  ALTER COLUMN source_ts_sec DROP NOT NULL,
  DROP CONSTRAINT video_moments_kind_check,
  ADD CONSTRAINT video_moments_kind_check CHECK (kind IN ('star', 'bad_coord', 'trim', 'boring'));
CREATE UNIQUE INDEX video_moments_key
  ON video_moments (video_id, kind, COALESCE(source_ts_sec, -1), username, platform);
