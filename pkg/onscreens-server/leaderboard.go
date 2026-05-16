package onscreensServer

import (
	"log/slog"
	"time"
)

var leaderboardDuration = time.Duration(20 * time.Second)

var Leaderboard *Onscreen

func InitLeaderboard() {
	slog.Info("creating onscreen", "kind", "leaderboard")
	Leaderboard = New()
}

func ShowLeaderboard(content string) {
	Leaderboard.ShowFor(content, leaderboardDuration)
}
