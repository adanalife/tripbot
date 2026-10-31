/* Seeds chatbot.find — the dormant-ship gate for the !find visual-search
   command (#879). Without this row the console toggle errors ("flag not
   found"), so !find can never be turned on.

   One row per platform (per the 019 guidance): !find is a playback command
   allowlisted on both the twitch and youtube instances, so each platform
   toggles independently.

   Defaults off: enable per env from the console once the video-pipeline embed
   responder is live there; flipping it back off reverts without a bot
   restart. */
INSERT INTO feature_flags (key, platform, description, enabled, target_removal_date)
VALUES
    (
        'chatbot.find',
        'twitch',
        'Enables the !find visual-search command: embeds the chat query via the video-pipeline embed responder and jumps the stream to the closest corpus moment. Requires the responder deployed in the env; toggling takes effect without a bot restart.',
        FALSE,
        DATE '2027-01-06'
    ),
    (
        'chatbot.find',
        'youtube',
        'Enables the !find visual-search command: embeds the chat query via the video-pipeline embed responder and jumps the stream to the closest corpus moment. Requires the responder deployed in the env; toggling takes effect without a bot restart.',
        FALSE,
        DATE '2027-01-06'
    )
ON CONFLICT (key, platform) DO NOTHING;
