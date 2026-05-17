package onscreensServer

import (
	"log/slog"
	"math/rand"
	"time"
)

var leftRotator *Onscreen

var leftRotatorUpdateFrequency = time.Duration(45 * time.Second)

var possibleLeftMessages = []string{
	"Crave something new? Try !timewarp",
	"Earn miles for every minute you watch (!miles)",
	"Follow the project elsewhere on !socialmedia",
	"Join us on !discord",
	"Join us on !discord",
	"Try and !guess what state we're in",
	"Use !commands to interact with the bot",
	"Use !commands to interact with the bot",
	"Where are we? (!location)",
	// "LEADER",
	// "Looking for artist for emotes and more",
	// "Twitch Prime subs keep us on air :D",
	// "Use !report to report stream issues",
}

func InitLeftRotator() {
	slog.Info("creating onscreen", "kind", "left-rotator")
	leftRotator = New()
	// Show a first message synchronously so the OBS browser source has
	// content to render the moment it polls — otherwise there's a brief
	// race where the rotator is empty until the goroutine schedules.
	leftRotator.Show(leftRotatorContent())
	go leftRotatorLoop()
}

func leftRotatorLoop() {
	for { // forever
		time.Sleep(time.Duration(leftRotatorUpdateFrequency))
		leftRotator.Show(leftRotatorContent())
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
