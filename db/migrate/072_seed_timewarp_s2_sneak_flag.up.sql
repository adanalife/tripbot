/* Seeds the flag that arms the season-2 sneak: with it on, a few viewer
   !timewarps land in the s2 corpus instead of the rotation.

   Off is the safe default. Until season-2 clips sit in playout's playlist a
   sneak has nothing to land on (playout falls back to the rotation), and once
   they do it is a surprise to switch on deliberately. One row per platform, per
   the 019 guidance. */
INSERT INTO feature_flags (key, platform, description, enabled, target_removal_date)
SELECT
    'chatbot.timewarp_s2_sneak',
    p.platform,
    'Sends 5% of viewer !timewarps into the season-2 footage instead of the usual rotation. Playout plays that block to its end, then returns to the rotation. !guess wins and gift warps never sneak.',
    FALSE,
    DATE '2027-04-08'
FROM (VALUES ('twitch'), ('youtube'), ('facebook'), ('instagram'), ('tiktok')) AS p(platform)
ON CONFLICT (key, platform) DO NOTHING;
