/* Per-platform rows (one per platform) per the 019 convention: each bot
   instance loads only the rows matching its STREAM_PLATFORM. Seeded FALSE so
   the credit stays off until verified on stream; enable per platform via the
   feature_flags table (Twitch first — YouTube can stay off). */
INSERT INTO feature_flags (key, platform, description, enabled, target_removal_date)
VALUES
    (
        'chatbot.timewarp_credit', 'twitch',
        'Shows the triggering chatter''s username as a credit line on the timewarp overlay (!timewarp / correct !guess). Enable per platform once verified on stream.',
        FALSE, DATE '2026-12-18'
    ),
    (
        'chatbot.timewarp_credit', 'youtube',
        'Shows the triggering chatter''s username as a credit line on the timewarp overlay (!timewarp / correct !guess). Enable per platform once verified on stream.',
        FALSE, DATE '2026-12-18'
    )
ON CONFLICT (key, platform) DO NOTHING;
