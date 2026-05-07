package onscreensServer

import (
	"log"
	"time"
)

var leaderboardDuration = time.Duration(20 * time.Second)

var Leaderboard *Onscreen

func InitLeaderboard() {
	log.Println("Creating leaderboard onscreen")
	Leaderboard = New()
}

func ShowLeaderboard(content string) {
	Leaderboard.ShowFor(content, leaderboardDuration)
}
