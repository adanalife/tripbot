/* Re-seed chatbot.twitch_gateway, enabled (its live state at removal time), so a
   rollback to a binary that still reads the flag routes through the gateway. */
INSERT INTO feature_flags (key, platform, description, enabled, target_removal_date)
VALUES (
    'chatbot.twitch_gateway',
    'twitch',
    'Routes the chatbot''s command-time Twitch Helix calls (e.g. !followage) through the platform-gateway twitch-api instead of the in-process pkg/twitch path. Enable once the gateway is verified on the env; disable to revert without a bot restart.',
    TRUE,
    DATE '2026-12-19'
)
ON CONFLICT (key, platform) DO NOTHING;
