/* Seeds the flags the TikTok gift work needs a console toggle for. A missing
   row is not "off but switchable" — Flags.Bool answers false for an unknown
   key AND the console toggle errors ("flag not found"), so the feature can
   never be turned on. One row per platform, per the 019 guidance.

   chatbot.gifts is new: the dormant-ship gate for gift-triggered effects. Only
   TikTok surfaces gifts today (its webcast is the one inbound stream carrying
   them), so it's the only platform that gets a row — a platform whose gateway
   adapter never emits a gift has nothing to toggle.

   chatbot.find gains rows for the platforms 032 predates. That migration
   seeded twitch + youtube because !find was Twitch-only with YouTube next;
   !find is now allowlisted on every gateway platform, and without these rows
   it is permanently dead on them.

   All default off. The embed responder must be live in the env before
   enabling find; toggling takes effect without a bot restart. */
INSERT INTO feature_flags (key, platform, description, enabled, target_removal_date)
VALUES
    (
        'chatbot.gifts',
        'tiktok',
        'Enables gift-triggered on-stream effects: a viewer gift fires the effect its value maps to (a timewarp today), crediting the gifter on the warp overlay. Gifts bypass the chat playback rate-limit but hold a short floor of their own.',
        FALSE,
        DATE '2027-01-26'
    ),
    (
        'chatbot.find',
        'tiktok',
        'Enables the !find visual-search command: embeds the chat query via the video-pipeline embed responder and jumps the stream to the closest corpus moment. Requires the responder deployed in the env; toggling takes effect without a bot restart.',
        FALSE,
        DATE '2027-01-06'
    ),
    (
        'chatbot.find',
        'facebook',
        'Enables the !find visual-search command: embeds the chat query via the video-pipeline embed responder and jumps the stream to the closest corpus moment. Requires the responder deployed in the env; toggling takes effect without a bot restart.',
        FALSE,
        DATE '2027-01-06'
    ),
    (
        'chatbot.find',
        'instagram',
        'Enables the !find visual-search command: embeds the chat query via the video-pipeline embed responder and jumps the stream to the closest corpus moment. Requires the responder deployed in the env; toggling takes effect without a bot restart.',
        FALSE,
        DATE '2027-01-06'
    )
ON CONFLICT (key, platform) DO NOTHING;
