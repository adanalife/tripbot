/* The scores dedupe is intentionally not reversed: merged rows can't be
   resurrected, and the summed value is the correct total either way. */
ALTER TABLE scores DROP CONSTRAINT scores_user_scoreboard_key;
ALTER TABLE scoreboards DROP CONSTRAINT scoreboards_name_platform_key;
ALTER TABLE scoreboards ADD CONSTRAINT scoreboards_name_key UNIQUE (name);
ALTER TABLE scoreboards DROP COLUMN platform;
