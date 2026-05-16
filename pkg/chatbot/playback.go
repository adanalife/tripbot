package chatbot

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	terrors "github.com/adanalife/tripbot/pkg/errors"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/helpers"
	"github.com/adanalife/tripbot/pkg/users"
	"github.com/adanalife/tripbot/pkg/video"
)

// lastTimewarpTime is used to rate-limit users so they can't
// over-do the time-skip features (including !skip and !back)
// plus it's also used to reset peoples lastLocation time
var lastTimewarpTime time.Time

// timewarp jumps the playhead to a random video in the loop
func (a *App) timewarp() {
	// show timewarp onscreen
	a.Onscreens.ShowTimewarp()

	// shuffle to a new video
	err := a.VLC.PlayRandom()
	if err != nil {
		terrors.Log(err, "error from VLC client")
	}
	// update the currently-playing video
	a.Video.GetCurrentlyPlaying()
	// update our record of last time it ran
	lastTimewarpTime = time.Now()
}

func (a *App) timewarpCmd(ctx context.Context, user *users.User, _ []string) {
	log.Println(user.Username, "ran !timewarp")

	// exit early if we're on OS X
	if helpers.RunningOnDarwin() {
		a.IRC.Say("Sorry, timewarp isn't available right now")
		return
	}

	// rate-limit the number of times this can run
	if !c.UserIsAdmin(user.Username) {
		if time.Now().Sub(lastTimewarpTime) < 20*time.Second {
			a.IRC.Say("Not yet; enjoy the moment!")
			return
		}
	}

	// only say this if the caller is not me
	if !c.UserIsAdmin(user.Username) {
		a.IRC.Say("Here we go...!")
	}

	// do the timewarp
	a.timewarp()
}

func (a *App) jumpCmd(ctx context.Context, user *users.User, params []string) {
	var err error
	log.Println(user.Username, "ran !jump")

	// exit early if we're on OS X
	if helpers.RunningOnDarwin() {
		a.IRC.Say("Sorry, jump isn't available right now")
		return
	}

	// rate-limit the number of times this can run
	if !c.UserIsAdmin(user.Username) {
		if time.Now().Sub(lastTimewarpTime) < 20*time.Second {
			a.IRC.Say("Not yet; enjoy the moment!")
			return
		}
	}

	// exit if the user gave no args or too many
	if len(params) == 0 || len(params) > 2 {
		a.IRC.Say("Usage: !jump [state]")
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
		a.IRC.Say(msg)
		return
	}
	// check to see if there was an error finding a candidate video
	if err != nil {
		terrors.Log(err, "error from finding random video for state")
		a.IRC.Say("Usage: !jump [state]")
		return
	}
	// tell VLC to play it
	err = a.VLC.PlayFileInPlaylist(randomVid.File())
	if err != nil {
		terrors.Log(err, "error from VLC client")
		a.IRC.Say("Usage: !jump [state]")
		return
	}
	a.IRC.Say(fmt.Sprintf("Jumping to %s...!", titlecaseState))
	// update the currently-playing video
	a.Video.GetCurrentlyPlaying()
	// show the flag for the state
	a.Onscreens.ShowFlag(10 * time.Second)
	// update our record of last time it ran
	lastTimewarpTime = time.Now()
}

func (a *App) skipCmd(ctx context.Context, user *users.User, params []string) {
	var err error
	var n int
	log.Println(user.Username, "ran !skip")

	// exit early if we're on OS X
	if helpers.RunningOnDarwin() {
		a.IRC.Say("Sorry, skip isn't available right now")
		return
	}

	// rate-limit the number of times this can run
	if !c.UserIsAdmin(user.Username) {
		if time.Now().Sub(lastTimewarpTime) < 20*time.Second {
			a.IRC.Say("Not yet; enjoy the moment!")
			return
		}
	}

	// first we count the given params
	if len(params) == 0 {
		// just skip once if no params
		n = 1
	} else {
		// we were given args
		// try and convert their input to a number
		n, err = strconv.Atoi(params[0])
		// if conversion fails or they give too many args
		if err != nil || len(params) > 1 {
			a.IRC.Say("Usage: !skip [num]")
			return
		}
	}

	// skip to a new video
	err = a.VLC.Skip(n)
	if err != nil {
		terrors.Log(err, "error from VLC client")
	}
	// update the currently-playing video
	a.Video.GetCurrentlyPlaying()
	// update our record of last time it ran
	lastTimewarpTime = time.Now()
}

func (a *App) backCmd(ctx context.Context, user *users.User, params []string) {
	var err error
	var n int
	log.Println(user.Username, "ran !back")

	// exit early if we're on OS X
	if helpers.RunningOnDarwin() {
		a.IRC.Say("Sorry, back isn't available right now")
		return
	}

	// rate-limit the number of times this can run
	if !c.UserIsAdmin(user.Username) {
		if time.Now().Sub(lastTimewarpTime) < 20*time.Second {
			a.IRC.Say("Not yet; enjoy the moment!")
			return
		}
	}

	// first we count the given params
	if len(params) == 0 {
		// just back once if no params
		n = 1
	} else {
		// we were given args
		// try and convert their input to a number
		n, err = strconv.Atoi(params[0])
		// if conversion fails or they give too many args
		if err != nil || len(params) > 1 {
			a.IRC.Say("Usage: !back [num]")
			return
		}
	}

	// back to an old video
	err = a.VLC.Back(n)
	if err != nil {
		terrors.Log(err, "error from VLC client")
	}
	// update the currently-playing video
	a.Video.GetCurrentlyPlaying()
	// update our record of last time it ran
	lastTimewarpTime = time.Now()
}
