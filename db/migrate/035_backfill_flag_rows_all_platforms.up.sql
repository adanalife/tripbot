/* Backfills every feature flag onto every platform.

   019 established "one row per platform" and seeded youtube by copying
   twitch's rows. facebook, instagram, and tiktok were added as platforms
   afterwards and never got that treatment, so each carries only the rows a
   later migration named explicitly. A platform missing a row doesn't inherit a
   default — Flags.Bool answers false for a key it can't find, so the feature
   is off with no way to switch it on.

   That's how chatbot.timewarp_credit ended up unreachable on tiktok: the warp
   overlay's credit line is gated on a row that was never created there.

   Copies from twitch (the platform that has every key) rather than naming
   keys, so this stays correct as flags come and go. enabled is deliberately
   NOT copied: a flag's on/off state is a per-platform operational decision,
   and inheriting twitch's would silently switch features on elsewhere. Every
   backfilled row lands disabled, matching how a newly seeded flag ships.
   ON CONFLICT keeps existing rows and their state untouched. */
INSERT INTO feature_flags
    (key, platform, description, enabled, enabled_for_usernames,
     enabled_for_roles, target_removal_date)
SELECT src.key, p.platform, src.description, FALSE, src.enabled_for_usernames,
       src.enabled_for_roles, src.target_removal_date
FROM feature_flags src
CROSS JOIN (VALUES ('youtube'), ('facebook'), ('instagram'), ('tiktok')) AS p(platform)
WHERE src.platform = 'twitch'
ON CONFLICT (key, platform) DO NOTHING;
