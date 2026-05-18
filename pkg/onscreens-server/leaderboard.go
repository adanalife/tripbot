package onscreensServer

import (
	"log/slog"
	"time"
)

var leaderboardDuration = time.Duration(20 * time.Second)

var leaderboard *Onscreen

func InitLeaderboard() {
	slog.Info("creating onscreen", "kind", "leaderboard")
	leaderboard = New()
}

func ShowLeaderboard(content string) {
	leaderboard.ShowFor(content, leaderboardDuration)
}
