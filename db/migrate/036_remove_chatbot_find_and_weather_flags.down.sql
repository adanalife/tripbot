/* Re-seed both flags ENABLED on every platform the commands are allowlisted on,
   so a rollback to a binary that still reads them keeps !find and !weather
   working rather than silently dead. That's the opposite of how they were
   originally seeded (off, pending verification) and deliberately so: the state
   worth restoring is the one they were retired from, which is on.

   chatbot.find carries rows for every gateway platform (per 034), chatbot.weather
   for twitch + youtube only — the platforms 017/019 gave it. A platform without
   a row evaluates false, which for weather matches the state at removal time. */
INSERT INTO feature_flags (key, platform, description, enabled, target_removal_date)
VALUES
    (
        'chatbot.find', 'twitch',
        'Enables the !find visual-search command: embeds the chat query via the video-pipeline embed responder and jumps the stream to the closest corpus moment.',
        TRUE, DATE '2027-01-06'
    ),
    (
        'chatbot.find', 'youtube',
        'Enables the !find visual-search command: embeds the chat query via the video-pipeline embed responder and jumps the stream to the closest corpus moment.',
        TRUE, DATE '2027-01-06'
    ),
    (
        'chatbot.find', 'tiktok',
        'Enables the !find visual-search command: embeds the chat query via the video-pipeline embed responder and jumps the stream to the closest corpus moment.',
        TRUE, DATE '2027-01-06'
    ),
    (
        'chatbot.find', 'facebook',
        'Enables the !find visual-search command: embeds the chat query via the video-pipeline embed responder and jumps the stream to the closest corpus moment.',
        TRUE, DATE '2027-01-06'
    ),
    (
        'chatbot.find', 'instagram',
        'Enables the !find visual-search command: embeds the chat query via the video-pipeline embed responder and jumps the stream to the closest corpus moment.',
        TRUE, DATE '2027-01-06'
    ),
    (
        'chatbot.weather', 'twitch',
        'Gates the !weather chat command (historical conditions at the dashcam location, via the Open-Meteo archive).',
        TRUE, DATE '2026-12-02'
    ),
    (
        'chatbot.weather', 'youtube',
        'Gates the !weather chat command (historical conditions at the dashcam location, via the Open-Meteo archive).',
        TRUE, DATE '2026-12-02'
    )
ON CONFLICT (key, platform) DO NOTHING;
