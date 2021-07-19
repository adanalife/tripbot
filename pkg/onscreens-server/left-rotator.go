package onscreensServer

import (
	"crypto/rand"
	"log"
	"path/filepath"
	"time"

	c "github.com/adanalife/tripbot/pkg/config/vlc-server"
)

var LeftRotator *Onscreen

var leftRotatorUpdateFrequency = time.Duration(45 * time.Second)

var leftRotatorFile = filepath.Join(c.Conf.RunDir, "left-message.txt")

var possibleLeftMessages = []string{
	"Crave something new? Try !timewarp",
	"Earn miles for every minute you watch (!miles)",
	"Follow the project elsewhere on !socialmedia",
	"Join us on !discord",
	"Join us on !discord",
	"Try and !guess what state we're in",
	"Use !commands to interact with the bot",
	"Use !commands to interact with the bot",
	"Want to help the project? Fill out the !survey",
	"Want to help the project? Fill out the !survey",
	"Where are we? (!location)",
	// "LEADER",
	// "Looking for artist for emotes and more",
	// "Twitch Prime subs keep us on air :D",
	// "Use !report to report stream issues",
}

func InitLeftRotator() {
	log.Println("Creating left rotator onscreen")
	LeftRotator = New(leftRotatorFile)
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
	if rand.Int(10000) == 0 {
		return "You found the rare message! Make a clip for a prize!"
	}

	// pick a random message
	message := possibleLeftMessages[rand.Intn(len(possibleLeftMessages))]

	// some messages require custom logic
	switch message {
	//case "LEADER":
	//	//TODO: maybe turn this into a call to tripbot?
	//	if len(users.Leaderboard) == 0 {
	//		terrors.Log(errors.New("leaderboard empty"), "")
	//		// just use the default value
	//		output = message
	//		break
	//	}
	//	// get the first leader in the leaderboard
	//	leader := users.Leaderboard[:1][0]
	//	output = fmt.Sprintf("%s is leader with %s miles (!leaderboard)", leader[0], leader[1])
	default:
		output = message
	}

	return output
}
