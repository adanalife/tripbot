package onscreensServer

import (
	"log"
	"path/filepath"
	"time"

	c "github.com/adanalife/tripbot/pkg/config/vlc-server"
)

var leaderboardDuration = time.Duration(20 * time.Second)
var leaderboardFile = filepath.Join(c.Conf.RunDir, "leaderboard.txt")

var Leaderboard *Onscreen

func InitLeaderboard() {
	log.Println("Creating leaderboard onscreen")
	Leaderboard = New(leaderboardFile)
}

func ShowLeaderboard(content string) {
	Leaderboard.ShowFor(content, leaderboardDuration)
}
