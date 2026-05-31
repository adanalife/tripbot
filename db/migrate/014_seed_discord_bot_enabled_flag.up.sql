INSERT INTO feature_flags (key, description, enabled, target_removal_date)
VALUES (
    'discord.bot_enabled',
    'Gates pkg/discord startup. Enable per env once the Discord app + bot token + guild ID are wired.',
    FALSE,
    DATE '2026-11-28'
)
ON CONFLICT (key) DO NOTHING;
