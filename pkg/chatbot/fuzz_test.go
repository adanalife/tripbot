package chatbot

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/adanalife/tripbot/pkg/video"
)

// splitFuzzParams mirrors how findCommand constructs params from a chat
// message: strings.Split(msg, " ") after trimming the command word. It
// preserves empty elements (so "a  b" yields ["a","","b"]) the same way the
// production path does, which is the surface we want fuzz to exercise.
func splitFuzzParams(s string) []string {
	return strings.Split(s, " ")
}

// newFuzzApp returns a test App whose IRC is a recording fake (not noopIRC,
// which delegates to the package-level sayFn and would dereference the nil
// twitch client during fuzz runs).
func newFuzzApp(vid video.Video) *App {
	app := newTestApp(vid)
	app.IRC = &recordingIRC{}
	return app
}

// FuzzFindCommand asserts the chat-message parser never panics on arbitrary
// input. Anything that gets typed into Twitch chat (including the invisible
// Chatterino dup-suppression rune \U000e0000, inverted-bang aliases, multi-
// word aliases, and arbitrary UTF-8) flows through findCommand.
func FuzzFindCommand(f *testing.F) {
	seeds := []string{
		"",
		" ",
		"!help",
		"!",
		"! help",
		"!miles @dana",
		"¡miles",
		"hi",
		"no audio",
		"frozen since yesterday",
		"!help \U000e0000",
		"!help\t",
		"\x00",
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		_, _ = builtTestApp.findCommand(s)
	})
}

// FuzzGuessCmd fuzzes the !guess argument parser. The video's State is set
// to a sentinel string no realistic chat input can match, so fuzz stays on
// the wrong-guess path (no DB writes). The handful of inputs that happen to
// match are skipped to avoid the AddToScore path's sqlmock requirement.
func FuzzGuessCmd(f *testing.F) {
	const unguessableState = "\x00ZZ_FUZZ_SENTINEL_ZZ\x00"

	seeds := []string{"", " ", "CA", "ca", "California", "@CA", "CA ZZ", "\x00", "  CA  "}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		app := newFuzzApp(newTestVideo(unguessableState, 0, 0, time.Time{}))
		params := splitFuzzParams(s)
		// guessCmd's correct-guess branch lower-cases both sides and the
		// 2-letter form gets expanded via helpers.StateAbbrevToState; bail
		// if either form matches the sentinel (essentially impossible).
		guess := strings.Join(params, " ")
		if strings.ToLower(guess) == strings.ToLower(unguessableState) {
			t.Skip("fuzz hit the sentinel state")
		}
		app.guessCmd(context.Background(), newTestUser("viewer1"), params)
	})
}

// FuzzMilesCmd exercises the other-user lookup path, which calls
// helpers.StripAtSign(params[0]). The noopSessions fake returns a zero-value
// user so milesCmd short-circuits before any DB-backed mileage lookup.
func FuzzMilesCmd(f *testing.F) {
	seeds := []string{"@dana", "dana", "", "@", " ", "@dana extra", "\x00", "@@dana"}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		app := newFuzzApp(video.Video{})
		params := splitFuzzParams(s)
		if len(params) == 0 {
			t.Skip("empty params hits the self path which reads CurrentMiles")
		}
		app.milesCmd(context.Background(), newTestUser("viewer1"), params)
	})
}

// FuzzFollowageCmd covers the StripAtSign-on-params[0] path for !followage.
// mytwitch.FollowedAt short-circuits when the broadcaster token isn't loaded
// (the test-env default), so no HTTP fires.
func FuzzFollowageCmd(f *testing.F) {
	seeds := []string{"@dana", "dana", "", "@", " ", "@dana extra", "\x00"}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		app := newFuzzApp(video.Video{})
		app.middleCmd(context.Background(), newTestUser("viewer1"), splitFuzzParams(s))
		app.followageCmd(context.Background(), newTestUser("viewer1"), splitFuzzParams(s))
	})
}

// FuzzMiddleCmd covers the admin-gated "hide" branch (single-param case-
// insensitive match) and the show-text branch (params joined by space). The
// recordingOnscreens fake swallows the overlay calls.
func FuzzMiddleCmd(f *testing.F) {
	seeds := []string{"hide", "HIDE", "Hide", "hello world", "", "  ", "hide me", "\x00", "@hide"}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		app := newFuzzApp(video.Video{})
		params := splitFuzzParams(s)
		app.middleCmd(context.Background(), newTestUser(adminUser), params)
	})
}

// FuzzSetBotFlag covers the !makebot / !unbot argument handling: lowercase
// + TrimPrefix("@") on params[0], then the noopSessions.SetBot call.
func FuzzSetBotFlag(f *testing.F) {
	seeds := []string{"@dana", "dana", "DANA", "@", "", " ", "@@dana", "@dana extra", "\x00"}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		app := newFuzzApp(video.Video{})
		params := splitFuzzParams(s)
		app.makeBotCmd(context.Background(), newTestUser(adminUser), params)
		app.unBotCmd(context.Background(), newTestUser(adminUser), params)
	})
}
