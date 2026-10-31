/* Seeds chatbot.guessr_leaderboard — the gate on the two guessing-game boards
   in the onscreen leaderboard rotation. Without this row the console toggle
   errors ("flag not found") and the boards can never be turned on, since
   Flags.Bool answers false for a key it can't find.

   One row per platform (per the 019 guidance), even though the leaderboard
   jobs are twitchOnlyJobs today: 035 exists because platforms added later
   silently missed rows and the feature became unreachable there. Cheaper to
   seed all five now than to notice the gap when the job scope widens.

   Defaults off. The boards read from guessr.dana.lol, a service outside this
   cluster, and land on a live broadcast — so the useful property is being able
   to take them off screen from the console without a deploy. Off, the rotation
   is exactly the three boards that predate the game. */
INSERT INTO feature_flags (key, platform, description, enabled, target_removal_date)
SELECT
    'chatbot.guessr_leaderboard',
    p.platform,
    'Enables the guessr daily + monthly boards in the onscreen leaderboard rotation. The rows are fetched from the game''s public API at guessr.dana.lol; off, the rotation is the three miles/guess boards only. Toggling takes effect on the next 5-minute tick, without a bot restart.',
    FALSE,
    DATE '2027-02-02'
FROM (VALUES ('twitch'), ('youtube'), ('facebook'), ('instagram'), ('tiktok')) AS p(platform)
ON CONFLICT (key, platform) DO NOTHING;
