/* Seeds chatbot.twitch_gateway — the runtime kill-switch for routing the Twitch
   bot's command-time Helix calls through the platform-gateway (twitch-api). The
   code-level flag landed in #904; without this row the console toggle errors
   ("flag not found"), so the gateway path can never be turned on.

   Twitch-only: the flag is consulted solely by the twitch instance (the youtube
   instance has no gateway wired, so flaggedTwitch is never constructed there).
   Unlike the 019 "one row per platform" guidance there is deliberately no
   youtube row — toggling it there would do nothing.

   Defaults off: an env wired with TWITCH_API_URL stays in-process until this is
   flipped on from the console, and flipping it back off reverts without a bot
   restart. */
INSERT INTO feature_flags (key, platform, description, enabled, target_removal_date)
VALUES (
    'chatbot.twitch_gateway',
    'twitch',
    'Routes the chatbot''s command-time Twitch Helix calls (e.g. !followage) through the platform-gateway twitch-api instead of the in-process pkg/twitch path. Enable once the gateway is verified on the env; disable to revert without a bot restart.',
    FALSE,
    DATE '2026-12-19'
)
ON CONFLICT (key, platform) DO NOTHING;
