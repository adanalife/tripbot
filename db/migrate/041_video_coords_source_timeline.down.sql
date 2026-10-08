DROP INDEX video_coords_aired_idx;
DELETE FROM video_coords WHERE ts_sec IS NULL;
ALTER TABLE video_coords ALTER COLUMN ts_sec SET NOT NULL;
ALTER TABLE video_coords DROP CONSTRAINT video_coords_pkey;
ALTER TABLE video_coords ADD PRIMARY KEY (video_id, ts_sec);
ALTER TABLE video_coords DROP COLUMN source_ts_sec;
