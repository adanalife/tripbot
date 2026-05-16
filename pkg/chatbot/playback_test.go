package chatbot

import (
	"context"
	"testing"
	"time"

	"github.com/adanalife/tripbot/pkg/video"
)

// These tests cover the four playback commands that refresh pkg/video state
// after telling vlc-server to change tracks. Before App.Video was injectable,
// the refresh was an unobserved package-level call into video.GetCurrentlyPlaying
// (which in turn hit vlc-server over HTTP). Now we can assert it fires.
//
// The *Cmd handlers early-return on Darwin via helpers.RunningOnDarwin(); the
// canonical test invocation is `task test` (Linux container, ENV=testing), so
// the Darwin branch is not exercised here.

// runAsAdmin runs fn with lastTimewarpTime cleared so rate limiting is not a
// concern, plus captureSay's restore wired up automatically. The admin
// shortcut still goes through helpers.RunningOnDarwin, but in CI that's false.
func runAsAdmin(t *testing.T, fn func()) {
	t.Helper()
	lastTimewarpTime = time.Time{}
	_, restore := captureSay(t)
	defer restore()
	fn()
}

// --- timewarpCmd ---

func TestTimewarpCmd_AdminDrivesPlaybackChain(t *testing.T) {
	app := newTestApp(video.Video{})
	recOverlay := &recordingOnscreens{}
	recVLC := &recordingVLC{}
	recVideo := &recordingVideo{}
	app.Onscreens = recOverlay
	app.VLC = recVLC
	app.Video = recVideo

	runAsAdmin(t, func() {
		app.timewarpCmd(context.Background(), newTestUser(adminUser), nil)
	})

	// Overlay: ShowTimewarp is the only call.
	if len(recOverlay.Calls) != 1 || recOverlay.Calls[0] != "ShowTimewarp()" {
		t.Errorf("expected one ShowTimewarp overlay call, got %v", recOverlay.Calls)
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

// --- skipCmd ---

func TestSkipCmd_AdminDrivesPlaybackChain(t *testing.T) {
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

func TestSkipCmd_AdminPassesParamCountThrough(t *testing.T) {
	app := newTestApp(video.Video{})
	recVLC := &recordingVLC{}
	recVideo := &recordingVideo{}
	app.VLC = recVLC
	app.Video = recVideo

	runAsAdmin(t, func() {
		app.skipCmd(context.Background(), newTestUser(adminUser), []string{"3"})
	})

	if len(recVLC.Calls) != 1 || recVLC.Calls[0] != "Skip(3)" {
		t.Errorf("expected one Skip(3) VLC call, got %v", recVLC.Calls)
	}
	if len(recVideo.Calls) != 1 || recVideo.Calls[0] != "GetCurrentlyPlaying()" {
		t.Errorf("expected one GetCurrentlyPlaying call on Video, got %v", recVideo.Calls)
	}
}

// --- backCmd ---

func TestBackCmd_AdminDrivesPlaybackChain(t *testing.T) {
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
	recVideo := &recordingVideo{}
	app.Video = recVideo

	expectAddToScoreChain(mock)
	expectAddToScoreChain(mock)

	_, restore := captureSay(t)
	defer restore()

	app.guessCmd(context.Background(), newTestUser("viewer1"), []string{"Colorado"})

	if len(recVideo.Calls) != 1 || recVideo.Calls[0] != "GetCurrentlyPlaying()" {
		t.Errorf("expected timewarp to refresh Video via GetCurrentlyPlaying, got %v", recVideo.Calls)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}
