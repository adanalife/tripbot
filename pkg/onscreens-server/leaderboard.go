package onscreensServer

import (
	"log/slog"
	"time"
)

var leaderboardDuration = time.Duration(20 * time.Second)

var leaderboard *Onscreen

func InitLeaderboard() {
	leaderboard = newLeaderboardOnscreen()
}

// newLeaderboardOnscreen constructs the leaderboard *Onscreen and emits
// the matching "creating onscreen" slog line for parity with the legacy
// InitX free functions.
func newLeaderboardOnscreen() *Onscreen {
	slog.Info("creating onscreen", "kind", "leaderboard")
	return newOnscreen()
}

func ShowLeaderboard(content string) {
	leaderboard.ShowFor(content, leaderboardDuration)
}
