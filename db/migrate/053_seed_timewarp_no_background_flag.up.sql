/* Seeds the flag that strips the warp overlay's opaque background, so it can
   be toggled live from the console while we judge it on stream.

   Off means the cover stays — that's the current behavior, and the safe one:
   the cover exists to mask the H.264 discontinuity the playhead jump causes,
   so an unreachable flag row (or a flag client that fell back to the
   in-memory default) must not strip it. One row per platform, per the 019
   guidance. */
INSERT INTO feature_flags (key, platform, description, enabled, target_removal_date)
SELECT
    'chatbot.timewarp_no_background',
    p.platform,
    'Removes the opaque cover behind the TIMEWARP overlay, leaving the wordmark and speed-lines over the live video. The cover normally masks the video gap the playhead jump causes, so expect to see that gap with this on.',
    FALSE,
    DATE '2027-03-19'
FROM (VALUES ('twitch'), ('youtube'), ('facebook'), ('instagram'), ('tiktok')) AS p(platform)
ON CONFLICT (key, platform) DO NOTHING;
