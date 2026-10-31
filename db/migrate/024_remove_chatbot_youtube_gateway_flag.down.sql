/* Re-seeds the chatbot.youtube_gateway flag (disabled), restoring 023's state
   so a downgrade lands on a tripbot build that still consults the flag. */
INSERT INTO feature_flags (key, platform, description, enabled, target_removal_date)
VALUES (
    'chatbot.youtube_gateway',
    'youtube',
    'Routes a youtube instance''s outbound chat sends through the platform-gateway gateway-youtube instead of the in-process pkg/youtube path. The inbound chat poll stays in-process. Enable once the gateway is verified on the env; disable to revert without a bot restart.',
    FALSE,
    DATE '2026-12-19'
)
ON CONFLICT (key, platform) DO NOTHING;
