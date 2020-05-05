package background

import (
	"fmt"
	"log"
	"math/rand"
	"path"
	"time"

	"github.com/dmerrick/tripbot/pkg/config"
	"github.com/dmerrick/tripbot/pkg/onscreens"
	"github.com/dmerrick/tripbot/pkg/users"
)

var LeftRotator *onscreens.Onscreen

var leftRotatorUpdateFrequency = time.Duration(45 * time.Second)

var leftRotatorFile = path.Join(config.RunDir, "left-message.txt")

var possibleLeftMessages = []string{
	"Looking for artist for emotes and more",
	"Want to help the stream? Fill out the !survey",
	"Want to help the stream? Fill out the !survey",
	// "Twitch Prime subs keep us on air :D",
	"Earn miles for every minute you watch (!miles)",
	// "I won't be offended if you play your own music",
	// "Music by Soma.fm (!song)",
	"Use !report to report stream issues",
	"Try and !guess what state we're in",
	"Where are we? (!location)",
	"Crave something new? Try !timewarp",
	"Find me on !socialmedia",
	"New here? Use !commands to interact with the bot",
	"LEADER",
}

func InitLeftRotator() {
	log.Println("Creating left rotator onscreen")
	LeftRotator = onscreens.New(leftRotatorFile)
	go leftRotatorLoop()
}

func leftRotatorLoop() {
	for { // forever
		LeftRotator.Show(leftRotatorContent())
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
