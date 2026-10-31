DELETE FROM video_moments WHERE kind = 'trim' OR username <> '';
ALTER TABLE video_moments
  DROP CONSTRAINT video_moments_pkey,
  ADD PRIMARY KEY (video_id, kind, source_ts_sec),
  DROP CONSTRAINT video_moments_kind_check,
  ADD CONSTRAINT video_moments_kind_check CHECK (kind IN ('star', 'bad_coord')),
  DROP COLUMN platform,
  DROP COLUMN username;
