package chatbot

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	terrors "github.com/adanalife/tripbot/pkg/errors"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/helpers"
	"github.com/adanalife/tripbot/pkg/users"
)

// lastTimewarpTime is used to rate-limit users so they can't
// over-do the time-skip features (including !skip and !back)
// plus it's also used to reset peoples lastLocation time
var lastTimewarpTime time.Time

// timewarpOverlayLeadIn is how long we wait after the "Here we go...!" chat
// message before bringing up the warp overlay. Lets the audience register the
// chat beat before the cover slams in, so the takeover feels intentional
// rather than instantaneous.
var timewarpOverlayLeadIn = 500 * time.Millisecond

// timewarpCoverDelay is how long we wait after triggering the full-screen warp
// overlay before actually jumping the playhead. The overlay is driven by a
// browser source that polls for state, so it needs a beat to bring the opaque
// cover up; the jump is a hard cut that makes OBS clear the dashcam layer, and
// the cover has to be in place to mask that gap.
var timewarpCoverDelay = 800 * time.Millisecond

// timewarp jumps the playhead to a random video in the loop
func (a *App) timewarp(ctx context.Context) {
	// give the chat message a beat to land before the visual takeover
	time.Sleep(timewarpOverlayLeadIn)

	// bring up the full-screen warp overlay, then give the browser source a
	// beat to render the opaque cover before we hard-cut to a new clip
	a.Onscreens.ShowTimewarp(ctx)
	time.Sleep(timewarpCoverDelay)

	// shuffle to a new video
	err := a.VLC.PlayRandom(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "error from VLC client", "err", err)
	}
	// update the currently-playing video
	a.Video.GetCurrentlyPlaying(ctx)
	// update our record of last time it ran
	lastTimewarpTime = time.Now()
}

func (a *App) timewarpCmd(ctx context.Context, user *users.User, _ []string) {
	slog.InfoContext(ctx, "ran !timewarp", "username", user.Username)

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
	a.timewarp(ctx)
}

func (a *App) jumpCmd(ctx context.Context, user *users.User, params []string) {
	var err error
	slog.InfoContext(ctx, "ran !jump", "username", user.Username)

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
	randomVid, err := a.Video.FindRandomByState(ctx, state)
	// check to see if we even have footage for this state
	if _, ok := err.(*terrors.NoFootageForStateError); ok {
		msg := fmt.Sprintf("No footage for %s... yet! ;)", titlecaseState)
		a.IRC.Say(msg)
		return
	}
	// check to see if there was an error finding a candidate video
	if err != nil {
		slog.ErrorContext(ctx, "error from finding random video for state", "err", err)
		a.IRC.Say("Usage: !jump [state]")
		return
	}
	// tell VLC to play it
	err = a.VLC.PlayFileInPlaylist(ctx, randomVid.File())
	if err != nil {
		slog.ErrorContext(ctx, "error from VLC client", "err", err)
		a.IRC.Say("Usage: !jump [state]")
		return
	}
	a.IRC.Say(fmt.Sprintf("Jumping to %s...!", titlecaseState))
	// update the currently-playing video
	a.Video.GetCurrentlyPlaying(ctx)
	// show the flag for the state
	a.Onscreens.ShowFlag(ctx, 10 * time.Second)
	// update our record of last time it ran
	lastTimewarpTime = time.Now()
}

func (a *App) skipCmd(ctx context.Context, user *users.User, params []string) {
	var err error
	var n int
	slog.InfoContext(ctx, "ran !skip", "username", user.Username)

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
	err = a.VLC.Skip(ctx, n)
	if err != nil {
		slog.ErrorContext(ctx, "error from VLC client", "err", err)
	}
	// update the currently-playing video
	a.Video.GetCurrentlyPlaying(ctx)
	// update our record of last time it ran
	lastTimewarpTime = time.Now()
}

func (a *App) backCmd(ctx context.Context, user *users.User, params []string) {
	var err error
	var n int
	slog.InfoContext(ctx, "ran !back", "username", user.Username)

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
	err = a.VLC.Back(ctx, n)
	if err != nil {
		slog.ErrorContext(ctx, "error from VLC client", "err", err)
	}
	// update the currently-playing video
	a.Video.GetCurrentlyPlaying(ctx)
	// update our record of last time it ran
	lastTimewarpTime = time.Now()
}
