-- Opt an account out of the leaderboards without calling it a bot.
--
-- is_bot drives real behavior (logout messages, human/bot counts, chat
-- greetings), so it can't double as "hide from the leaderboard": the channel
-- owner's own account belongs on the human side of every one of those and
-- still shouldn't be ranked against viewers. Leaderboard membership gets its
-- own flag; both signals exclude, independently.
ALTER TABLE users
  ADD COLUMN exclude_from_leaderboard BOOLEAN NOT NULL DEFAULT FALSE;

-- The two accounts known to need it. A no-op on any environment where they
-- don't exist.
UPDATE users
   SET exclude_from_leaderboard = TRUE
 WHERE username IN ('adanalife_', 'tripbot4000');
