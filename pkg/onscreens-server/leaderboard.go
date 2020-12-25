package onscreensServer

import (
	"log"
	"path/filepath"
	"time"

	"github.com/adanalife/tripbot/pkg/config"
)

var leaderboardDuration = time.Duration(20 * time.Second)
var leaderboardFile = filepath.Join(config.RunDir, "leaderboard.txt")

var Leaderboard *Onscreen

func InitLeaderboard() {
	log.Println("Creating leaderboard onscreen")
	Leaderboard = New(leaderboardFile)
	// go leaderboardLoop()
}

func ShowLeaderboard(content string) {

	//TODO: re-enable this
	log.Println("Not showing leaderboard")
	// Leaderboard.ShowFor(content, leaderboardDuration)
}

// func leaderboardLoop() {
// 	for { // forever
// 		if rand.Intn(10) == 0 {
// 			ShowLeaderboard()
// 		}
// 		time.Sleep(time.Duration(30 * time.Second))
// 	}
// }
