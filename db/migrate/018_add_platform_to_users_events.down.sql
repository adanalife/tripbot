/* Fails if the same username exists on multiple platforms — collapse those
   rows manually before downgrading. */
ALTER TABLE events DROP COLUMN platform;

ALTER TABLE users DROP CONSTRAINT users_platform_username_key;
ALTER TABLE users ADD CONSTRAINT users_username_key UNIQUE (username);
ALTER TABLE users DROP COLUMN platform;
