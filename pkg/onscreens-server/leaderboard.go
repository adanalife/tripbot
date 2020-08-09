package onscreensServer

import (
	"fmt"
	"log"
	"math/rand"
	"path"
	"time"

	"github.com/adanalife/tripbot/pkg/config"
	"github.com/adanalife/tripbot/pkg/users"
)

var leaderboardDuration = time.Duration(20 * time.Second)
var leaderboardFile = path.Join(config.RunDir, "leaderboard.txt")

var Leaderboard *Onscreen

func InitLeaderboard() {
	log.Println("Creating leaderboard onscreen")
	Leaderboard = New(leaderboardFile)
	go leaderboardLoop()
}

func ShowLeaderboard() {
	Leaderboard.ShowFor(leaderboardContent(), leaderboardDuration)
}

func leaderboardLoop() {
	for { // forever
		if rand.Intn(10) == 0 {
			ShowLeaderboard()
		}
		time.Sleep(time.Duration(30 * time.Second))
	}
}

// leaderboardContent creates the content for the leaderboard
func leaderboardContent() string {
	var output string
	output = "Odometer Leaderboard\n"

	size := 5
	if len(users.Leaderboard) < size {
		size = len(users.Leaderboard)
	}
	leaderboard := users.Leaderboard[:size]

	for _, score := range leaderboard {
		output = output + fmt.Sprintf("%s miles: %s\n", score[1], score[0])
	}

	return output
}
