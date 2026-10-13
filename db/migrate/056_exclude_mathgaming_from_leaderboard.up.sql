-- mathgaming is the channel owner's personal account, so it stays off the
-- leaderboards alongside adanalife_ and tripbot4000. A no-op on any
-- environment where the account doesn't exist.
UPDATE users
   SET exclude_from_leaderboard = TRUE
 WHERE username = 'mathgaming';
