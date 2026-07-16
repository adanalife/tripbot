package chatbot

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	terrors "github.com/adanalife/tripbot/pkg/errors"

	"github.com/adanalife/tripbot/pkg/feature"
	"github.com/adanalife/tripbot/pkg/helpers"
	"github.com/adanalife/tripbot/pkg/users"
)

// timewarpCreditFlagKey gates the on-overlay username credit (the chatter who
// triggered !timewarp or a correct !guess). Defaults off — the flag row is
// seeded FALSE per platform; flip it on per env / platform once the credit is
// verified on stream. The warp itself always runs; only the credit is gated.
const timewarpCreditFlagKey = "chatbot.timewarp_credit"

// lastTimewarpTime is used to rate-limit users so they can't
// over-do the time-skip features (including !skip and !back)
// plus it's also used to reset peoples lastLocation time
var lastTimewarpTime time.Time

// timewarpCoverDelay is how long we wait after triggering the full-screen warp
// overlay before actually jumping the playhead. The overlay is driven by a
// browser source that polls for state, so it needs a beat to bring the opaque
// cover up; the jump is a hard cut that makes OBS clear the dashcam layer, and
// the cover has to be in place to mask that gap.
var timewarpCoverDelay = 800 * time.Millisecond

// showTimewarpOverlay brings up the full-screen warp overlay that masks a
// playhead jump: it resolves the (feature-flagged) username credit, triggers
// the overlay, then waits for the browser source to render the opaque cover
// before the caller hard-cuts.
// Shared by !timewarp/!guess (random jump) and !find (targeted jump).
func (a *App) showTimewarpOverlay(ctx context.Context, username string) {
	// The on-overlay username credit is feature-flagged (per platform); the
	// warp always runs, only the credit line is gated. An empty credit means
	// the overlay shows no @-line.
	credit := username
	if credit != "" && !a.Flags.Bool(ctx, timewarpCreditFlagKey, feature.EvalContext{
		Username: username,
		Channel:  a.Cfg.ChannelName,
		Env:      a.Cfg.Environment,
	}) {
		credit = ""
	}

	// bring up the full-screen warp overlay, then give the browser source a
	// beat to render the opaque cover before we hard-cut
	a.Onscreens.ShowTimewarp(ctx, credit)
	time.Sleep(timewarpCoverDelay)
}

// timewarp jumps the playhead to a random video in the loop. username is the
// chatter who triggered it — surfaced as a credit line on the warp overlay
// (empty for callers with no attributable user).
func (a *App) timewarp(ctx context.Context, username string) {
	a.showTimewarpOverlay(ctx, username)

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
		a.Chat.Say("Sorry, timewarp isn't available right now")
		return
	}

	// rate-limit the number of times this can run
	if !a.Cfg.UserIsAdmin(user.Username) {
		if time.Now().Sub(lastTimewarpTime) < 20*time.Second {
			a.Chat.Say("Not yet; enjoy the moment!")
			return
		}
	}

	// only say this if the caller is not me
	if !a.Cfg.UserIsAdmin(user.Username) {
		a.Chat.Say("Here we go...!")
	}

	// do the timewarp, crediting the caller on the overlay
	a.timewarp(ctx, user.Username)
}

func (a *App) jumpCmd(ctx context.Context, user *users.User, params []string) {
	var err error
	slog.InfoContext(ctx, "ran !jump", "username", user.Username)

	// exit early if we're on OS X
	if helpers.RunningOnDarwin() {
		a.Chat.Say("Sorry, jump isn't available right now")
		return
	}

	// rate-limit the number of times this can run
	if !a.Cfg.UserIsAdmin(user.Username) {
		if time.Now().Sub(lastTimewarpTime) < 20*time.Second {
			a.Chat.Say("Not yet; enjoy the moment!")
			return
		}
	}

	// exit if the user gave no args or too many
	if len(params) == 0 || len(params) > 2 {
		a.Chat.Say("Usage: !jump [state]")
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
		a.Chat.Say(msg)
		return
	}
	// check to see if there was an error finding a candidate video
	if err != nil {
		slog.ErrorContext(ctx, "error from finding random video for state", "err", err)
		a.Chat.Say("Usage: !jump [state]")
		return
	}
	// tell VLC to play it
	err = a.VLC.PlayFileInPlaylist(ctx, randomVid.File())
	if err != nil {
		slog.ErrorContext(ctx, "error from VLC client", "err", err)
		a.Chat.Say("Usage: !jump [state]")
		return
	}
	a.Chat.Say(fmt.Sprintf("Jumping to %s...!", titlecaseState))
	// update the currently-playing video
	a.Video.GetCurrentlyPlaying(ctx)
	// update our record of last time it ran
	lastTimewarpTime = time.Now()
}

// seekSpanMax caps how far one !skip/!back can move the playhead. Bigger
// jumps are what !timewarp and !goto are for, and each seek walks clip
// durations server-side — an unbounded span would walk the whole corpus.
const seekSpanMax = 24 * time.Hour

// parseSeekSpan turns a !skip/!back argument into a footage duration.
// Accepts Go duration forms ("10m", "1h30m", "90s") and bare numbers, which
// mean minutes ("!skip 10" moves ten minutes). The sign comes back as given —
// "!skip -10m" is a rewind.
func parseSeekSpan(arg string) (time.Duration, error) {
	if n, err := strconv.Atoi(arg); err == nil {
		return time.Duration(n) * time.Minute, nil
	}
	return time.ParseDuration(arg)
}

// formatSeekSpan renders a span the way a chatter would type it: "10m",
// "1h30m", "45s" — no trailing zero units (time.Duration.String would give
// "1h30m0s"). Callers pass a positive span.
func formatSeekSpan(d time.Duration) string {
	d = d.Round(time.Second)
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	var b strings.Builder
	if h > 0 {
		fmt.Fprintf(&b, "%dh", h)
	}
	if m > 0 {
		fmt.Fprintf(&b, "%dm", m)
	}
	if s > 0 || b.Len() == 0 {
		fmt.Fprintf(&b, "%ds", s)
	}
	return b.String()
}

func (a *App) skipCmd(ctx context.Context, user *users.User, params []string) {
	a.seekCmd(ctx, user, params, "!skip", 1)
}

func (a *App) backCmd(ctx context.Context, user *users.User, params []string) {
	a.seekCmd(ctx, user, params, "!back", -1)
}

// seekCmd is the shared !skip/!back handler; dir is +1 for !skip, -1 for
// !back. Without an argument it hops one whole clip in dir's direction. With
// one it moves the playhead by that span of footage ("!skip 10m",
// "!back 1h30m", bare numbers meaning minutes), crossing clip boundaries as
// needed. A negative span flips direction, so "!skip -10m" rewinds.
func (a *App) seekCmd(ctx context.Context, user *users.User, params []string, name string, dir int) {
	slog.InfoContext(ctx, "ran "+name, "username", user.Username)

	// exit early if we're on OS X
	if helpers.RunningOnDarwin() {
		a.Chat.Say(fmt.Sprintf("Sorry, %s isn't available right now", strings.TrimPrefix(name, "!")))
		return
	}

	// rate-limit the number of times this can run
	if !a.Cfg.UserIsAdmin(user.Username) {
		if time.Now().Sub(lastTimewarpTime) < 20*time.Second {
			a.Chat.Say("Not yet; enjoy the moment!")
			return
		}
	}

	if len(params) == 0 {
		// no argument: hop a single clip
		var err error
		if dir >= 0 {
			err = a.VLC.Skip(ctx, 1)
		} else {
			err = a.VLC.Back(ctx, 1)
		}
		if err != nil {
			slog.ErrorContext(ctx, "error from VLC client", "err", err)
		}
	} else {
		// joining params lets "!skip 1h 30m" read as one span
		span, err := parseSeekSpan(strings.Join(params, ""))
		if err != nil || span == 0 {
			a.Chat.Say(fmt.Sprintf("Usage: %s [time, like 10m or 1h30m]", name))
			return
		}
		if span > seekSpanMax || span < -seekSpanMax {
			a.Chat.Say(fmt.Sprintf("%s tops out at 24h", name))
			return
		}
		span *= time.Duration(dir)
		if err := a.VLC.Seek(ctx, span); err != nil {
			slog.ErrorContext(ctx, "error from VLC client", "err", err)
		}
		if span > 0 {
			a.Chat.Say("⏩ Skipping ahead " + formatSeekSpan(span))
		} else {
			a.Chat.Say("⏪ Going back " + formatSeekSpan(-span))
		}
	}

	// update the currently-playing video
	a.Video.GetCurrentlyPlaying(ctx)
	// update our record of last time it ran
	lastTimewarpTime = time.Now()
}
