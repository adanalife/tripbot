`CHANNEL_NAME` is lowercased once when `TripbotConfig` loads, so the scoreboard and leaderboard queries no longer wrap `cfg.ChannelName` in `strings.ToLower` at each use site.
