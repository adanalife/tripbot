package background

import (
	"fmt"
	"math/rand"
	"path"
	"time"

	"github.com/dmerrick/danalol-stream/pkg/helpers"
	"github.com/dmerrick/danalol-stream/pkg/onscreens"
	"github.com/dmerrick/danalol-stream/pkg/users"
)

var LeftRotator *onscreens.Onscreen

var leftRotatorUpdateFrequency = time.Duration(45 * time.Second)

// super long duration cause this is always on
var leftRotatorDuration = time.Duration(24 * time.Hour)
var leftRotatorFile = path.Join(helpers.ProjectRoot(), "OBS/left-message.txt")

var possibleLeftMessages = []string{
	"Looking for artist for emotes and more",
	"Want to help the stream? Fill out the !survey",
	"Want to help the stream? Fill out the !survey",
	"Want to help the stream? Fill out the !survey",
	"Twitch Prime subs keep us on air :D",
	"Earn miles for every minute you watch (!miles)",
	"I won't be offended if you play your own music",
	"Music by Soma.fm (!song)",
	"Use !report to report stream issues",
	"Try and !guess what state we're in",
	"Where are we? (!location)",
	"LEADER",
}

func InitLeftRotator() {
	LeftRotator = onscreens.New(leftRotatorFile)
	go leftRotatorLoop()
}

func leftRotatorLoop() {
	for { // forever
		LeftRotator.Show(leftRotatorContent(), leftRotatorDuration)
		time.Sleep(time.Duration(leftRotatorUpdateFrequency))
	}
}

// leftRotatorContent creates the content for the leftRotator
func leftRotatorContent() string {
	var output string

	// show a special, very rare message
	if rand.Intn(10000) == 0 {
		return "You found the rare message! Make a clip for a prize!"
	}

	// pick a random message
	message := possibleLeftMessages[rand.Intn(len(possibleLeftMessages))]

	// some messages require custom logic
	switch message {
	case "LEADER":
		// get the first leader in the leaderboard
		leader := users.Leaderboard[:1][0]
		output = fmt.Sprintf("%s is leader with %s miles (!leaderboard)", leader[0], leader[1])
	default:
		output = message
	}

	return output
}