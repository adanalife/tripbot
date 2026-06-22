package onscreensServer

import (
	"log/slog"
	"math/rand"
	"time"
)

var leftRotatorUpdateFrequency = time.Duration(45 * time.Second)

// !miles and !guess are Twitch-only (not in the YouTube command allowlist), so
// those lines are scoped to Twitch — a YouTube overlay would otherwise advertise
// commands that silently no-op there. Weight 2 reproduces the old duplicated
// entries (!discord, !commands each appeared twice).
var possibleLeftMessages = []rotatorMessage{
	{Text: "Crave something new? Try `!timewarp`"},
	{Text: "Earn miles for every minute you watch (`!miles`)", Platforms: []string{platformTwitch}},
	{Text: "Follow the project elsewhere on `!socialmedia`"},
	{Text: "Join us on `!discord`", Weight: 2},
	{Text: "Try and `!guess` what state we're in", Platforms: []string{platformTwitch}},
	{Text: "Use `!commands` to interact with the bot", Weight: 2},
	{Text: "Where are we? (`!location`)"},
	// {Text: "LEADER"},
	// {Text: "Looking for artist for emotes and more"},
	// {Text: "Twitch Prime subs keep us on air :D"},
	// {Text: "Use !report to report stream issues"},
}

// botlessLeftMessages replace the command-hint left rotator on a bot-less
// YouTube instance: no commands work there, so point viewers at the live,
// interactive Twitch stream and signal that YouTube interactivity is coming.
var botlessLeftMessages = []rotatorMessage{
	{Text: "Chat with the bot live on Twitch", Weight: 2},
	{Text: "twitch.tv/ADanaLife_", Weight: 2},
	{Text: "Interactive commands coming to YouTube soon"},
	{Text: "Follow the journey live on Twitch"},
}

// newLeftRotator constructs the left-rotator *Onscreen, primes it with a
// first message synchronously (so the OBS browser source has content to
// render the moment it polls — otherwise there's a brief race where the
// rotator is empty until the goroutine schedules), and kicks off the
// background loop that rotates the message every leftRotatorUpdateFrequency.
func newLeftRotator() *Onscreen {
	slog.Info("creating onscreen", "kind", "left-rotator")
	osc := newOnscreen()
	osc.Show(leftRotatorContent())
	go leftRotatorLoop(osc)
	return osc
}

func leftRotatorLoop(osc *Onscreen) {
	for { // forever
		time.Sleep(time.Duration(leftRotatorUpdateFrequency))
		osc.Show(leftRotatorContent())
	}
}

// leftRotatorContent creates the content for the leftRotator
func leftRotatorContent() string {
	// show a special, very rare message
	if rand.Intn(10000) == 0 {
		return "You found the rare message! Make a clip for a prize!"
	}

	if botless() {
		return pickRotatorMessage(botlessLeftMessages)
	}

	// pick a weighted-random message for this platform
	return pickRotatorMessage(possibleLeftMessages)
}
