DELETE FROM feature_flags WHERE platform != 'twitch';

ALTER TABLE feature_flags DROP CONSTRAINT feature_flags_pkey;
ALTER TABLE feature_flags ADD PRIMARY KEY (key);
ALTER TABLE feature_flags DROP COLUMN platform;
