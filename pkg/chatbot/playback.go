package chatbot

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	terrors "github.com/adanalife/tripbot/pkg/errors"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/helpers"
	onscreensClient "github.com/adanalife/tripbot/pkg/onscreens-client"
	"github.com/adanalife/tripbot/pkg/users"
	"github.com/adanalife/tripbot/pkg/video"
	vlcClient "github.com/adanalife/tripbot/pkg/vlc-client"
)

// lastTimewarpTime is used to rate-limit users so they cant
// over-do the time-skip features (including !skip and !back)
// plus it's also used to reset peoples lastLocation time
var lastTimewarpTime time.Time

// timewarp jumps the playhead to a random video in the loop
func timewarp() {
	// show timewarp onscreen
	onscreensClient.ShowTimewarp()

	// shuffle to a new video
	err := vlcClient.PlayRandom()
	if err != nil {
		terrors.Log(err, "error from VLC client")
	}
	// update the currently-playing video
	video.GetCurrentlyPlaying()
	// update our record of last time it ran
	lastTimewarpTime = time.Now()
}

func timewarpCmd(user *users.User) {
	log.Println(user.Username, "ran !timewarp")

	// exit early if we're on OS X
	if helpers.RunningOnDarwin() {
		Say("Sorry, timewarp isn't available right now")
		return
	}

	// rate-limit the number of times this can run
	if !c.UserIsAdmin(user.Username) {
		if time.Now().Sub(lastTimewarpTime) < 20*time.Second {
			Say("Not yet; enjoy the moment!")
			return
		}
	}

	// only say this if the caller is not me
	if !c.UserIsAdmin(user.Username) {
		Say("Here we go...!")
	}

	// do the timewarp
	timewarp()
}

func jumpCmd(user *users.User, params []string) {
	var err error
	log.Println(user.Username, "ran !jump")

	// exit early if we're on OS X
	if helpers.RunningOnDarwin() {
		Say("Sorry, jump isn't available right now")
		return
	}

	// rate-limit the number of times this can run
	if !c.UserIsAdmin(user.Username) {
		if time.Now().Sub(lastTimewarpTime) < 20*time.Second {
			Say("Not yet; enjoy the moment!")
			return
		}
	}

	// exit if the user gave no args or too many
	if len(params) == 0 || len(params) > 2 {
		Say("Usage: !jump [state]")
		return
	}

	// skip to a video from the given state
	state := strings.Join(params, " ")
	// sanitize the input
	state = helpers.RemoveNonLetters(state)
	titlecaseState := helpers.TitlecaseState(state)
	randomVid, err := video.FindRandomByState(state)
	// check to see if we even have footage for this state
	if _, ok := err.(*terrors.NoFootageForStateError); ok {
		msg := fmt.Sprintf("No footage for %s... yet! ;)", titlecaseState)
		Say(msg)
		return
	}
	// check to see if there was an error finding a candidate video
	if err != nil {
		terrors.Log(err, "error from finding random video for state")
		Say("Usage: !jump [state]")
		return
	}
	// tell VLC to play it
	err = vlcClient.PlayFileInPlaylist(randomVid.File())
	if err != nil {
		terrors.Log(err, "error from VLC client")
		Say("Usage: !jump [state]")
		return
	}
	Say(fmt.Sprintf("Jumping to %s...!", titlecaseState))
	// update the currently-playing video
	video.GetCurrentlyPlaying()
	// show the flag for the state
	onscreensClient.ShowFlag(10 * time.Second)
	// update our record of last time it ran
	lastTimewarpTime = time.Now()
}

// parseSkipParams extracts the integer offset from chat-command params.
// Returns (n, ok). If ok is false, the caller should bail with a usage
// message — params were malformed (non-numeric, too many args, or empty
// string). An empty params list is valid and yields the supplied default.
func parseSkipParams(params []string, defaultN int) (int, bool) {
	if len(params) == 0 {
		return defaultN, true
	}
	if len(params) > 1 {
		return 0, false
	}
	n, err := strconv.Atoi(params[0])
	if err != nil {
		return 0, false
	}
	return n, true
}

func skipCmd(user *users.User, params []string) {
	var err error
	log.Println(user.Username, "ran !skip")

	// exit early if we're on OS X
	if helpers.RunningOnDarwin() {
		Say("Sorry, skip isn't available right now")
		return
	}

	// rate-limit the number of times this can run
	if !c.UserIsAdmin(user.Username) {
		if time.Now().Sub(lastTimewarpTime) < 20*time.Second {
			Say("Not yet; enjoy the moment!")
			return
		}
	}

	n, ok := parseSkipParams(params, 1)
	if !ok {
		Say("Usage: !skip [num]")
		return
	}

	// negative skip is equivalent to going back; route through Back
	// so the underlying client serializes the offset correctly
	if n < 0 {
		err = vlcClient.Back(-n)
	} else {
		err = vlcClient.Skip(n)
	}
	if err != nil {
		terrors.Log(err, "error from VLC client")
	}
	// update the currently-playing video
	video.GetCurrentlyPlaying()
	// update our record of last time it ran
	lastTimewarpTime = time.Now()
}

func backCmd(user *users.User, params []string) {
	var err error
	log.Println(user.Username, "ran !back")

	// exit early if we're on OS X
	if helpers.RunningOnDarwin() {
		Say("Sorry, back isn't available right now")
		return
	}

	// rate-limit the number of times this can run
	if !c.UserIsAdmin(user.Username) {
		if time.Now().Sub(lastTimewarpTime) < 20*time.Second {
			Say("Not yet; enjoy the moment!")
			return
		}
	}

	n, ok := parseSkipParams(params, 1)
	if !ok {
		Say("Usage: !back [num]")
		return
	}

	// negative back is equivalent to skipping forward; route through Skip
	// so the underlying client serializes the offset correctly
	if n < 0 {
		err = vlcClient.Skip(-n)
	} else {
		err = vlcClient.Back(n)
	}
	if err != nil {
		terrors.Log(err, "error from VLC client")
	}
	// update the currently-playing video
	video.GetCurrentlyPlaying()
	// update our record of last time it ran
	lastTimewarpTime = time.Now()
}
