DELETE FROM video_moments WHERE source_ts_sec IS NULL OR kind = 'boring';
DROP INDEX video_moments_key;
ALTER TABLE video_moments
  ALTER COLUMN source_ts_sec SET NOT NULL,
  ADD PRIMARY KEY (video_id, kind, source_ts_sec, username, platform),
  DROP CONSTRAINT video_moments_kind_check,
  ADD CONSTRAINT video_moments_kind_check CHECK (kind IN ('star', 'bad_coord', 'trim'));
