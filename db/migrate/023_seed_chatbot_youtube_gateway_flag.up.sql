/* Seeds chatbot.youtube_gateway — the runtime kill-switch for routing a
   PLATFORM=youtube instance's outbound chat sends through the platform-gateway
   (gateway-youtube). The code-level flag landed alongside the YOUTUBE_API_URL
   wiring; without this row the console toggle errors ("flag not found"), so the
   gateway path can never be turned on.

   YouTube-only: the flag is consulted solely by the youtube instance (the
   twitch instance has no YouTube gateway wired, so flaggedYouTubeSend is never
   constructed there). Seeded for the youtube platform to match the youtube
   instance's flag client (NewPostgresClient is constructed with STREAM_PLATFORM).

   Defaults off: an env wired with YOUTUBE_API_URL stays in-process until this is
   flipped on from the console, and flipping it back off reverts without a bot
   restart. Mirrors 021 (chatbot.twitch_gateway). */
INSERT INTO feature_flags (key, platform, description, enabled, target_removal_date)
VALUES (
    'chatbot.youtube_gateway',
    'youtube',
    'Routes a youtube instance''s outbound chat sends through the platform-gateway gateway-youtube instead of the in-process pkg/youtube path. The inbound chat poll stays in-process. Enable once the gateway is verified on the env; disable to revert without a bot restart.',
    FALSE,
    DATE '2026-12-19'
)
ON CONFLICT (key, platform) DO NOTHING;
