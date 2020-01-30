package background

import (
	"log"
	"math/rand"
	"path"
	"time"

	"github.com/dmerrick/danalol-stream/pkg/helpers"
	"github.com/dmerrick/danalol-stream/pkg/onscreens"
)

var RightRotator *onscreens.Onscreen

var rightRotatorUpdateFrequency = time.Duration(90 * time.Second)

// super long duration cause this is always on
var rightRotatorDuration = time.Duration(10 * 365 * 24 * time.Hour)
var rightRotatorFile = path.Join(helpers.ProjectRoot(), "OBS/right-message.txt")

var possibleRightMessages = []string{
	"Don't forget to follow :)",
	"Don't forget to follow :)",
	"Try running !location",
	"Try running !location",
	"Streaming 24 hours a day",
}

func InitRightRotator() {
	log.Println("Creating right rotator onscreen")
	RightRotator = onscreens.New(rightRotatorFile)
	go rightRotatorLoop()
}

func rightRotatorLoop() {
	for { // forever
		RightRotator.Show(rightRotatorContent(), rightRotatorDuration)
		time.Sleep(time.Duration(rightRotatorUpdateFrequency))
	}
}

// rightRotatorContent creates the content for the rightRotator
func rightRotatorContent() string {
	var output string

	// pick a random message
	output = possibleRightMessages[rand.Intn(len(possibleRightMessages))]

	return output
}
