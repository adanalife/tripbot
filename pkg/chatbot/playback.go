package chatbot

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	terrors "github.com/dmerrick/tripbot/pkg/errors"

	"github.com/dmerrick/tripbot/pkg/background"
	"github.com/dmerrick/tripbot/pkg/helpers"
	"github.com/dmerrick/tripbot/pkg/users"
	"github.com/dmerrick/tripbot/pkg/video"
	vlcClient "github.com/dmerrick/tripbot/pkg/vlc-client"
)

func timewarpCmd(user *users.User) {
	log.Println(user.Username, "ran !timewarp")

	// exit early if we're on OS X
	if helpers.RunningOnDarwin() {
		Say("Sorry, timewarp isn't available right now")
		return
	}

	// rate-limit the number of times this can run
	if !helpers.UserIsAdmin(user.Username) {
		if time.Now().Sub(lastTimewarpTime) < 20*time.Second {
			Say("Not yet; enjoy the moment!")
			return
		}
	}

	// show timewarp onscreen
	background.ShowTimewarp()

	// only say this if the caller is not me
	if !helpers.UserIsAdmin(user.Username) {
		Say("Here we go...!")
	}

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

func jumpCmd(user *users.User, params []string) {
	var err error
	log.Println(user.Username, "ran !jump")

	// exit early if we're on OS X
	if helpers.RunningOnDarwin() {
		Say("Sorry, jump isn't available right now")
		return
	}

	// rate-limit the number of times this can run
	if !helpers.UserIsAdmin(user.Username) {
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
	video.ShowFlag()
	// update our record of last time it ran
	lastTimewarpTime = time.Now()
}

func skipCmd(user *users.User, params []string) {
	var err error
	var n int
	log.Println(user.Username, "ran !skip")

	// exit early if we're on OS X
	if helpers.RunningOnDarwin() {
		Say("Sorry, skip isn't available right now")
		return
	}

	// rate-limit the number of times this can run
	if !helpers.UserIsAdmin(user.Username) {
		if time.Now().Sub(lastTimewarpTime) < 20*time.Second {
			Say("Not yet; enjoy the moment!")
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
			Say("Usage: !skip [num]")
			return
		}
	}

	// skip to a new video
	err = vlcClient.Skip(n)
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
	var n int
	log.Println(user.Username, "ran !back")

	// exit early if we're on OS X
	if helpers.RunningOnDarwin() {
		Say("Sorry, back isn't available right now")
		return
	}

	// rate-limit the number of times this can run
	if !helpers.UserIsAdmin(user.Username) {
		if time.Now().Sub(lastTimewarpTime) < 20*time.Second {
			Say("Not yet; enjoy the moment!")
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
			Say("Usage: !back [num]")
			return
		}
	}

	// back to an old video
	err = vlcClient.Back(n)
	if err != nil {
		terrors.Log(err, "error from VLC client")
	}
	// update the currently-playing video
	video.GetCurrentlyPlaying()
	// update our record of last time it ran
	lastTimewarpTime = time.Now()
}
