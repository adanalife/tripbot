/* Viewers mark moments from chat too: !favorite stars the moment on screen
   and !trim reports a stretch that wants cutting, each with an optional
   note. username and platform name who set a mark, empty for the console's
   own, and are part of the key so two viewers' marks on one second both
   stand. */
ALTER TABLE video_moments
  ADD COLUMN username TEXT NOT NULL DEFAULT '',
  ADD COLUMN platform TEXT NOT NULL DEFAULT '',
  DROP CONSTRAINT video_moments_kind_check,
  ADD CONSTRAINT video_moments_kind_check CHECK (kind IN ('star', 'bad_coord', 'trim')),
  DROP CONSTRAINT video_moments_pkey,
  ADD PRIMARY KEY (video_id, kind, source_ts_sec, username, platform);
