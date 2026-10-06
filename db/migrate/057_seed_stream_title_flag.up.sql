/* Seeds the flag that keeps each platform's stream title on the place on
   screen — "Driving through Bishop, California at golden hour" — written
   through the platform-gateway's metadata store every five minutes when it
   changes.

   Off by default: on, it overwrites whatever title the operator saved, so it
   is flipped per platform from the console. YouTube charges quota for each
   title write, so leave it off there until the quota extension lands. One row
   per platform, per the 019 guidance. */
INSERT INTO feature_flags (key, platform, description, enabled, target_removal_date)
SELECT
    'chatbot.stream_title',
    p.platform,
    'Sets the stream title to the place on screen and the time of day it was filmed, replacing the saved title whenever it changes (at most every five minutes). Off keeps the title you saved.',
    FALSE,
    DATE '2027-04-01'
FROM (VALUES ('twitch'), ('youtube'), ('facebook'), ('instagram'), ('tiktok')) AS p(platform)
ON CONFLICT (key, platform) DO NOTHING;
