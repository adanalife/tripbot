package onscreensServer

import (
	"log/slog"
	"math/rand"
	"time"
)

var RightRotator *Onscreen

var rightRotatorUpdateFrequency = time.Duration(90 * time.Second)

var possibleRightMessages = []string{
	"Don't forget to follow :)",
	"Don't forget to follow :)",
	"Try running !location",
	"Try running !location",
	"Try running !timewarp",
	"Streaming 24 hours a day",
}

func InitRightRotator() {
	slog.Info("creating onscreen", "kind", "right-rotator")
	RightRotator = New()
	// Show a first message synchronously so the OBS browser source has
	// content to render the moment it polls — otherwise there's a brief
	// race where the rotator is empty until the goroutine schedules.
	RightRotator.Show(rightRotatorContent())
	go rightRotatorLoop()
}

func rightRotatorLoop() {
	for { // forever
		time.Sleep(time.Duration(rightRotatorUpdateFrequency))
		RightRotator.Show(rightRotatorContent())
	}
}

// rightRotatorContent creates the content for the rightRotator
func rightRotatorContent() string {
	var output string

	// pick a random message
	output = possibleRightMessages[rand.Intn(len(possibleRightMessages))]

	return output
}
