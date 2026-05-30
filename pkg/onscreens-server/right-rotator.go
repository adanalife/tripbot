package onscreensServer

import (
	"log/slog"
	"math/rand"
	"time"
)

var rightRotatorUpdateFrequency = time.Duration(90 * time.Second)

var possibleRightMessages = []string{
	"Don't forget to follow :)",
	"Don't forget to follow :)",
	"Try running !location",
	"Try running !location",
	"Try running !timewarp",
	"Streaming 24 hours a day",
}

// newRightRotator constructs the right-rotator *Onscreen, primes it with
// a first message synchronously (so the OBS browser source has content
// to render the moment it polls — otherwise there's a brief race where
// the rotator is empty until the goroutine schedules), and kicks off the
// background loop that rotates the message every rightRotatorUpdateFrequency.
// The rotator loop exits when the onscreen's shutdown() is called.
func newRightRotator() *Onscreen {
	slog.Info("creating onscreen", "kind", "right-rotator")
	osc := newOnscreen()
	osc.Show(rightRotatorContent())
	osc.wg.Add(1)
	go rightRotatorLoop(osc)
	return osc
}

func rightRotatorLoop(osc *Onscreen) {
	defer osc.wg.Done()
	for {
		t := time.NewTimer(time.Duration(rightRotatorUpdateFrequency))
		select {
		case <-osc.stop:
			t.Stop()
			return
		case <-t.C:
		}
		osc.Show(rightRotatorContent())
	}
}

// rightRotatorContent creates the content for the rightRotator
func rightRotatorContent() string {
	var output string

	// pick a random message
	output = possibleRightMessages[rand.Intn(len(possibleRightMessages))]

	return output
}
