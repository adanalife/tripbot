package chatbot

import (
	"context"
	"runtime"
	"strings"
	"testing"
	"time"

	terrors "github.com/adanalife/tripbot/pkg/errors"
	"github.com/adanalife/tripbot/pkg/video"
)

// These tests cover the four playback commands that refresh pkg/video state
// after telling vlc-server to change tracks. Before App.Video was injectable,
// the refresh was an unobserved package-level call into video.GetCurrentlyPlaying
// (which in turn hit vlc-server over HTTP). Now we can assert it fires.
//
// The *Cmd handlers early-return on Darwin via helpers.RunningOnDarwin(), so
// each test calls skipIfDarwin to no-op when running `go test` locally on a Mac.
// The canonical test invocation is `task test` (Linux container, ENV=testing).

// skipIfDarwin no-ops the test when GOOS=darwin. The *Cmd handlers under test
// short-circuit on Darwin via helpers.RunningOnDarwin(), so the assertions below
// would never see the recording fakes get called.
func skipIfDarwin(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "darwin" {
		t.Skip("playback *Cmd handlers early-return on darwin; covered in CI (linux)")
	}
}

// runAsAdmin runs fn with lastTimewarpTime cleared so rate limiting is not a
// concern. Chat output goes to the App's IRC fake (noopChat by default).
func runAsAdmin(t *testing.T, fn func()) {
	t.Helper()
	lastTimewarpTime = time.Time{}
	fn()
}

// --- timewarpCmd ---

func TestTimewarpCmd_AdminDrivesPlaybackChain(t *testing.T) {
	skipIfDarwin(t)
	app := newTestApp(video.Video{})
	recOverlay := &recordingOnscreens{}
	recVLC := &recordingVLC{}
	recVideo := &recordingVideo{}
	app.Onscreens = recOverlay
	app.VLC = recVLC
	app.Video = recVideo
	// Credit flag on → the caller's username rides the overlay call.
	app.Flags = &recordingFlags{Set: map[string]bool{timewarpCreditFlagKey: true}}

	runAsAdmin(t, func() {
		app.timewarpCmd(context.Background(), newTestUser(adminUser), nil)
	})

	// Overlay: ShowTimewarp is the only call, crediting the caller.
	if len(recOverlay.Calls) != 1 || recOverlay.Calls[0] != `ShowTimewarp("test")` {
		t.Errorf("expected one ShowTimewarp overlay call crediting the caller, got %v", recOverlay.Calls)
	}
	// VLC: PlayRandom shuffles to a new video.
	if len(recVLC.Calls) != 1 || recVLC.Calls[0] != "PlayRandom()" {
		t.Errorf("expected one PlayRandom VLC call, got %v", recVLC.Calls)
	}
	// Video: GetCurrentlyPlaying refreshes pkg/video state after the shuffle.
	if len(recVideo.Calls) != 1 || recVideo.Calls[0] != "GetCurrentlyPlaying()" {
		t.Errorf("expected one GetCurrentlyPlaying call on Video, got %v", recVideo.Calls)
	}
}

// With the credit flag off (the default / fresh-deploy state via noopFlags),
// the warp still fires but the overlay gets no username — ShowTimewarp("").
func TestTimewarpCmd_CreditFlagOff_NoUsername(t *testing.T) {
	skipIfDarwin(t)
	app := newTestApp(video.Video{})
	recOverlay := &recordingOnscreens{}
	app.Onscreens = recOverlay
	app.VLC = &recordingVLC{}
	app.Video = &recordingVideo{}

	runAsAdmin(t, func() {
		app.timewarpCmd(context.Background(), newTestUser(adminUser), nil)
	})

	if len(recOverlay.Calls) != 1 || recOverlay.Calls[0] != `ShowTimewarp("")` {
		t.Errorf("expected ShowTimewarp with no credit, got %v", recOverlay.Calls)
	}
}

// --- skipCmd ---

func TestSkipCmd_AdminDrivesPlaybackChain(t *testing.T) {
	skipIfDarwin(t)
	app := newTestApp(video.Video{})
	recVLC := &recordingVLC{}
	recVideo := &recordingVideo{}
	app.VLC = recVLC
	app.Video = recVideo

	runAsAdmin(t, func() {
		// No params → n = 1.
		app.skipCmd(context.Background(), newTestUser(adminUser), nil)
	})

	if len(recVLC.Calls) != 1 || recVLC.Calls[0] != "Skip(1)" {
		t.Errorf("expected one Skip(1) VLC call, got %v", recVLC.Calls)
	}
	if len(recVideo.Calls) != 1 || recVideo.Calls[0] != "GetCurrentlyPlaying()" {
		t.Errorf("expected one GetCurrentlyPlaying call on Video, got %v", recVideo.Calls)
	}
}

// With an argument, !skip/!back move the playhead by a span of footage via
// Seek rather than hopping clips: durations parse as Go durations, bare
// numbers mean minutes, and the sign picks the direction ("!skip -10m"
// rewinds). The chat reply states the span moved.
func TestSkipAndBackCmd_SpansSeekByFootageDuration(t *testing.T) {
	skipIfDarwin(t)
	cases := []struct {
		name     string
		cmd      string
		params   []string
		wantCall string
		wantSay  string
	}{
		{"skip duration", "skip", []string{"10m"}, "Seek(10m0s)", "⏩ Skipping ahead 10 minutes"},
		{"skip bare number means minutes", "skip", []string{"3"}, "Seek(3m0s)", "⏩ Skipping ahead 3 minutes"},
		{"skip spaced span joins", "skip", []string{"1h", "30m"}, "Seek(1h30m0s)", "⏩ Skipping ahead 1 hour 30 minutes"},
		{"skip negative rewinds", "skip", []string{"-10m"}, "Seek(-10m0s)", "⏪ Going back 10 minutes"},
		{"back duration", "back", []string{"45s"}, "Seek(-45s)", "⏪ Going back 45 seconds"},
		{"back negative fast-forwards", "back", []string{"-2m"}, "Seek(2m0s)", "⏩ Skipping ahead 2 minutes"},
		{"any timescale goes through", "skip", []string{"1000h"}, "Seek(1000h0m0s)", "⏩ Skipping ahead 5 weeks 6 days"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app := newTestApp(video.Video{})
			recVLC := &recordingVLC{}
			recVideo := &recordingVideo{}
			recIRC := &recordingChat{}
			app.VLC = recVLC
			app.Video = recVideo
			app.Chat = recIRC

			runAsAdmin(t, func() {
				if tc.cmd == "skip" {
					app.skipCmd(context.Background(), newTestUser(adminUser), tc.params)
				} else {
					app.backCmd(context.Background(), newTestUser(adminUser), tc.params)
				}
			})

			if len(recVLC.Calls) != 1 || recVLC.Calls[0] != tc.wantCall {
				t.Errorf("expected one %s VLC call, got %v", tc.wantCall, recVLC.Calls)
			}
			if len(recVideo.Calls) != 1 || recVideo.Calls[0] != "GetCurrentlyPlaying()" {
				t.Errorf("expected one GetCurrentlyPlaying call on Video, got %v", recVideo.Calls)
			}
			if len(recIRC.Says) != 1 || recIRC.Says[0] != tc.wantSay {
				t.Errorf("expected reply %q, got %v", tc.wantSay, recIRC.Says)
			}
		})
	}
}

// Unparseable spans reply with usage without touching playback. Any
// parseable timescale is allowed (the player wraps modulo the corpus), so
// only spans that overflow time.Duration count as unparseable.
func TestSkipCmd_RejectsUnparseableSpans(t *testing.T) {
	skipIfDarwin(t)
	cases := []struct {
		name    string
		params  []string
		wantSay string
	}{
		{"gibberish", []string{"potato"}, "Usage: !skip [time, like 10m or 1h30m]"},
		{"zero span", []string{"0m"}, "Usage: !skip [time, like 10m or 1h30m]"},
		{"bare minutes overflowing a duration", []string{"99999999999999999"}, "Usage: !skip [time, like 10m or 1h30m]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app := newTestApp(video.Video{})
			recVLC := &recordingVLC{}
			recVideo := &recordingVideo{}
			recIRC := &recordingChat{}
			app.VLC = recVLC
			app.Video = recVideo
			app.Chat = recIRC

			runAsAdmin(t, func() {
				app.skipCmd(context.Background(), newTestUser(adminUser), tc.params)
			})

			if len(recVLC.Calls) != 0 {
				t.Errorf("expected no VLC calls, got %v", recVLC.Calls)
			}
			if len(recVideo.Calls) != 0 {
				t.Errorf("expected no Video calls, got %v", recVideo.Calls)
			}
			if len(recIRC.Says) != 1 || recIRC.Says[0] != tc.wantSay {
				t.Errorf("expected reply %q, got %v", tc.wantSay, recIRC.Says)
			}
		})
	}
}

// --- backCmd ---

func TestBackCmd_AdminDrivesPlaybackChain(t *testing.T) {
	skipIfDarwin(t)
	app := newTestApp(video.Video{})
	recVLC := &recordingVLC{}
	recVideo := &recordingVideo{}
	app.VLC = recVLC
	app.Video = recVideo

	runAsAdmin(t, func() {
		app.backCmd(context.Background(), newTestUser(adminUser), nil)
	})

	if len(recVLC.Calls) != 1 || recVLC.Calls[0] != "Back(1)" {
		t.Errorf("expected one Back(1) VLC call, got %v", recVLC.Calls)
	}
	if len(recVideo.Calls) != 1 || recVideo.Calls[0] != "GetCurrentlyPlaying()" {
		t.Errorf("expected one GetCurrentlyPlaying call on Video, got %v", recVideo.Calls)
	}
}

// --- guessCmd (correct guess re-exercise) ---
//
// The correct-guess path in commands.go calls a.timewarp() internally, which
// now goes through a.Video.GetCurrentlyPlaying() instead of the package-level
// call. Re-asserting here makes the Video injection a first-class concern of
// the correct-guess chain rather than just a side effect.

func TestGuessCmd_CorrectGuess_RefreshesVideoAfterTimewarp(t *testing.T) {
	mock := installMockDB(t)
	vid := newTestVideo("Colorado", 39.5, -105.0, time.Now())
	app := newTestApp(vid)
	recVideo := &recordingVideo{Vid: vid}
	app.Video = recVideo

	expectAddToScoreChain(mock)
	expectAddToScoreChain(mock)

	app.guessCmd(context.Background(), newTestUser("viewer1"), []string{"Colorado"})

	// guessCmd first reads the current vid (Current), then the correct-guess
	// path runs a.timewarp() which refreshes via GetCurrentlyPlaying.
	wantCalls := []string{"Current()", "GetCurrentlyPlaying()"}
	if len(recVideo.Calls) != len(wantCalls) ||
		recVideo.Calls[0] != wantCalls[0] || recVideo.Calls[1] != wantCalls[1] {
		t.Errorf("expected calls %v, got %v", wantCalls, recVideo.Calls)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

// --- jumpCmd ---
//
// jumpCmd was previously untestable because it called the package-level
// video.FindRandomByState directly (DB-backed). With Video.FindRandomByState
// on the injectable Video interface, we can stage results and exercise all
// three branches: success, no-footage-for-state, and bad input.

func TestJumpCmd_AdminPlaysRandomFromState(t *testing.T) {
	skipIfDarwin(t)
	app := newTestApp(video.Video{})
	recOverlay := &recordingOnscreens{}
	recVLC := &recordingVLC{}
	recVideo := &recordingVideo{
		// staged result returned from FindRandomByState; .File() renders as
		// "<Slug>.MP4" — that's what gets passed to VLC.PlayFileInPlaylist.
		RandomVid: video.Video{Slug: "2019_0615_183000_001", State: "California"},
	}
	recIRC := &recordingChat{}
	app.Onscreens = recOverlay
	app.VLC = recVLC
	app.Video = recVideo
	app.Chat = recIRC

	runAsAdmin(t, func() {
		app.jumpCmd(context.Background(), newTestUser(adminUser), []string{"california"})
	})

	// Video: FindRandomByState("california") then GetCurrentlyPlaying() after VLC handoff.
	wantVideo := []string{`FindRandomByState("california")`, "GetCurrentlyPlaying()"}
	if len(recVideo.Calls) != len(wantVideo) {
		t.Fatalf("expected %d Video calls, got %d: %v", len(wantVideo), len(recVideo.Calls), recVideo.Calls)
	}
	for i, want := range wantVideo {
		if recVideo.Calls[i] != want {
			t.Errorf("Video call %d: want %q, got %q", i, want, recVideo.Calls[i])
		}
	}

	// VLC: PlayFileInPlaylist called with the staged video's filename.
	wantVLC := `PlayFileInPlaylist("2019_0615_183000_001.MP4")`
	if len(recVLC.Calls) != 1 || recVLC.Calls[0] != wantVLC {
		t.Errorf("expected one %s VLC call, got %v", wantVLC, recVLC.Calls)
	}

	// Onscreens: !jump drives no overlay.
	if len(recOverlay.Calls) != 0 {
		t.Errorf("expected no overlay calls, got %v", recOverlay.Calls)
	}

	// IRC: a "Jumping to California...!" message.
	if len(recIRC.Says) != 1 || !strings.Contains(recIRC.Says[0], "Jumping to California") {
		t.Errorf("expected single 'Jumping to California' message, got %v", recIRC.Says)
	}
}

func TestJumpCmd_NoFootageForState(t *testing.T) {
	skipIfDarwin(t)
	app := newTestApp(video.Video{})
	recOverlay := &recordingOnscreens{}
	recVLC := &recordingVLC{}
	recVideo := &recordingVideo{
		RandomErr: &terrors.NoFootageForStateError{Msg: "no matches found"},
	}
	recIRC := &recordingChat{}
	app.Onscreens = recOverlay
	app.VLC = recVLC
	app.Video = recVideo
	app.Chat = recIRC

	runAsAdmin(t, func() {
		app.jumpCmd(context.Background(), newTestUser(adminUser), []string{"wyoming"})
	})

	// FindRandomByState was called; no GetCurrentlyPlaying refresh after.
	if len(recVideo.Calls) != 1 || recVideo.Calls[0] != `FindRandomByState("wyoming")` {
		t.Errorf("expected single FindRandomByState(\"wyoming\"), got %v", recVideo.Calls)
	}

	// No VLC handoff, no overlay.
	if len(recVLC.Calls) != 0 {
		t.Errorf("expected no VLC calls on no-footage path, got %v", recVLC.Calls)
	}
	if len(recOverlay.Calls) != 0 {
		t.Errorf("expected no overlay calls on no-footage path, got %v", recOverlay.Calls)
	}

	// IRC: the "No footage for X... yet!" message (titlecased).
	if len(recIRC.Says) != 1 || !strings.Contains(recIRC.Says[0], "No footage for Wyoming") {
		t.Errorf("expected single 'No footage for Wyoming' message, got %v", recIRC.Says)
	}
}

func TestJumpCmd_RejectsBadInput(t *testing.T) {
	skipIfDarwin(t)
	app := newTestApp(video.Video{})
	recVLC := &recordingVLC{}
	recVideo := &recordingVideo{}
	recIRC := &recordingChat{}
	app.VLC = recVLC
	app.Video = recVideo
	app.Chat = recIRC

	runAsAdmin(t, func() {
		app.jumpCmd(context.Background(), newTestUser(adminUser), nil)
	})

	// No state lookup, no playback.
	if len(recVideo.Calls) != 0 {
		t.Errorf("expected no Video calls on bad input, got %v", recVideo.Calls)
	}
	if len(recVLC.Calls) != 0 {
		t.Errorf("expected no VLC calls on bad input, got %v", recVLC.Calls)
	}

	// IRC: usage message.
	if len(recIRC.Says) != 1 || !strings.Contains(recIRC.Says[0], "Usage: !jump") {
		t.Errorf("expected usage message via IRC, got %v", recIRC.Says)
	}
}
