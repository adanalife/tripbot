/* Per-platform feature flags: each bot instance loads and toggles only the
   rows matching its STREAM_PLATFORM, so enabling a flag on YouTube doesn't
   enable it on Twitch. Existing rows are all Twitch; seed YouTube copies so
   both platforms start with the same flag set (same defaults). Future flag
   seed migrations must insert one row per platform. */
ALTER TABLE feature_flags ADD COLUMN platform TEXT NOT NULL DEFAULT 'twitch';
ALTER TABLE feature_flags DROP CONSTRAINT feature_flags_pkey;
ALTER TABLE feature_flags ADD PRIMARY KEY (key, platform);

INSERT INTO feature_flags
    (key, platform, description, enabled, enabled_for_usernames,
     enabled_for_roles, target_removal_date)
SELECT key, 'youtube', description, enabled, enabled_for_usernames,
       enabled_for_roles, target_removal_date
FROM feature_flags
WHERE platform = 'twitch'
ON CONFLICT (key, platform) DO NOTHING;
