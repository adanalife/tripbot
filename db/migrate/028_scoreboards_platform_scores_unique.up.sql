/* Monthly scoreboards were platform-blind: names are global (miles_2026_07),
   so once a second platform writes scores, both platforms' rows land on one
   board and mix in reads. Same move as feature_flags (019): scope the row by
   platform, all existing rows are Twitch. */
ALTER TABLE scoreboards ADD COLUMN platform TEXT NOT NULL DEFAULT 'twitch';
ALTER TABLE scoreboards DROP CONSTRAINT scoreboards_name_key;
ALTER TABLE scoreboards ADD CONSTRAINT scoreboards_name_platform_key UNIQUE (name, platform);

/* scores never had a uniqueness guarantee on (user_id, scoreboard_id), and
   find-or-create races created duplicate pairs (27 known in prod as of
   2026-06-12). The dup rows hold independent increments from those races,
   so the correct merge is to SUM into the keeper (lowest id), then drop the rest
   and add the constraint the race needed all along. */
UPDATE scores s SET value = agg.total
FROM (SELECT MIN(id) AS keep_id, SUM(value) AS total
      FROM scores GROUP BY user_id, scoreboard_id HAVING COUNT(*) > 1) agg
WHERE s.id = agg.keep_id;

DELETE FROM scores s
USING (SELECT id, ROW_NUMBER() OVER (PARTITION BY user_id, scoreboard_id ORDER BY id) AS rn
       FROM scores) d
WHERE s.id = d.id AND d.rn > 1;

ALTER TABLE scores ADD CONSTRAINT scores_user_scoreboard_key UNIQUE (user_id, scoreboard_id);
