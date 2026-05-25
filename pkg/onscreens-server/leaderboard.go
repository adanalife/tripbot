package onscreensServer

import (
	"log/slog"
	"time"
)

// leaderboardDuration controls how long a !leaderboard render stays on
// screen before the background expiry sweeper hides it.
var leaderboardDuration = time.Duration(20 * time.Second)

// newLeaderboardOnscreen constructs the leaderboard *Onscreen.
func newLeaderboardOnscreen() *Onscreen {
	slog.Info("creating onscreen", "kind", "leaderboard")
	return newOnscreen()
}
