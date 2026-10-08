/* Per-platform viewer identity: a YouTube viewer named "foo" and a Twitch
   viewer named "foo" are different people. Existing rows are all Twitch. */
ALTER TABLE users ADD COLUMN platform TEXT NOT NULL DEFAULT 'twitch';
ALTER TABLE users DROP CONSTRAINT users_username_key;
ALTER TABLE users ADD CONSTRAINT users_platform_username_key UNIQUE (platform, username);

ALTER TABLE events ADD COLUMN platform TEXT NOT NULL DEFAULT 'twitch';
