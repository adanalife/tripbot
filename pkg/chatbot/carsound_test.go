package chatbot

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/adanalife/tripbot/pkg/video"
)

// --- App.OBS seam fakes ---

// noopOBS swallows every OBS call — the default in newTestApp.
type noopOBS struct{}

func (noopOBS) SetBackgroundAudioFile(context.Context, string, string) error { return nil }
func (noopOBS) RefreshBrowserSources(context.Context) (int, error)           { return 0, nil }

// recordingOBS captures SetBackgroundAudioFile calls so tests can assert on the
// input name + file path, and can be primed to return an error.
type recordingOBS struct {
	Calls      []string // "<inputName>|<file>" per call
	Refreshed  int      // count returned from RefreshBrowserSources
	refreshErr error
	err        error
}

func (r *recordingOBS) SetBackgroundAudioFile(_ context.Context, inputName, file string) error {
	r.Calls = append(r.Calls, inputName+"|"+file)
	return r.err
}

func (r *recordingOBS) RefreshBrowserSources(context.Context) (int, error) {
	return r.Refreshed, r.refreshErr
}

func TestCarSoundCmd_NoArgReportsCurrent_NoSwitch(t *testing.T) {
	app := newTestApp(video.Video{})
	rec := &recordingChat{}
	app.Chat = rec
	obs := &recordingOBS{}
	app.OBS = obs

	app.carSoundCmd(context.Background(), newTestUser("viewer1"), nil)

	if len(obs.Calls) != 0 {
		t.Fatalf("no-arg !carsound should not switch OBS, got calls: %v", obs.Calls)
	}
	if len(rec.Says) != 1 || !strings.Contains(rec.Says[0], carSounds[0].Name) {
		t.Fatalf("expected report naming the default sound %q, got %v", carSounds[0].Name, rec.Says)
	}
}

func TestCarSoundCmd_NextCyclesAndAnnounces(t *testing.T) {
	app := newTestApp(video.Video{})
	rec := &recordingChat{}
	app.Chat = rec
	obs := &recordingOBS{}
	app.OBS = obs

	// Default index is 0 (idle); next -> 1 (highway).
	app.carSoundCmd(context.Background(), newTestUser("viewer1"), []string{"next"})

	want := carHumInputName + "|" + carSounds[1].file()
	if len(obs.Calls) != 1 || obs.Calls[0] != want {
		t.Fatalf("expected one OBS call %q, got %v", want, obs.Calls)
	}
	if app.carSoundIdx != 1 {
		t.Errorf("expected carSoundIdx advanced to 1, got %d", app.carSoundIdx)
	}
	if len(rec.Says) != 1 || !strings.Contains(rec.Says[0], carSounds[1].Name) {
		t.Errorf("expected announcement naming %q, got %v", carSounds[1].Name, rec.Says)
	}
}

func TestCarSoundCmd_NextWrapsAround(t *testing.T) {
	app := newTestApp(video.Video{})
	app.Chat = &recordingChat{}
	app.OBS = &recordingOBS{}
	app.carSoundIdx = len(carSounds) - 1

	app.carSoundCmd(context.Background(), newTestUser("viewer1"), []string{"next"})

	if app.carSoundIdx != 0 {
		t.Errorf("next from the last sound should wrap to 0, got %d", app.carSoundIdx)
	}
}

func TestCarSoundCmd_NamedJump(t *testing.T) {
	app := newTestApp(video.Video{})
	app.Chat = &recordingChat{}
	obs := &recordingOBS{}
	app.OBS = obs

	app.carSoundCmd(context.Background(), newTestUser("viewer1"), []string{"Mountain"}) // case-insensitive

	idx := findCarSound("mountain")
	want := carHumInputName + "|" + carSounds[idx].file()
	if len(obs.Calls) != 1 || obs.Calls[0] != want {
		t.Fatalf("expected OBS call %q, got %v", want, obs.Calls)
	}
	if app.carSoundIdx != idx {
		t.Errorf("expected carSoundIdx %d, got %d", idx, app.carSoundIdx)
	}
}

func TestCarSoundCmd_UnknownNameNoSwitch(t *testing.T) {
	app := newTestApp(video.Video{})
	rec := &recordingChat{}
	app.Chat = rec
	obs := &recordingOBS{}
	app.OBS = obs

	app.carSoundCmd(context.Background(), newTestUser("viewer1"), []string{"spaceship"})

	if len(obs.Calls) != 0 {
		t.Fatalf("unknown sound should not switch OBS, got %v", obs.Calls)
	}
	if app.carSoundIdx != 0 {
		t.Errorf("unknown sound should leave the index unchanged, got %d", app.carSoundIdx)
	}
	if len(rec.Says) != 1 || !strings.Contains(rec.Says[0], "No car sound") {
		t.Errorf("expected an unknown-sound reply, got %v", rec.Says)
	}
}

func TestCarSoundCmd_OBSErrorKeepsIndex(t *testing.T) {
	app := newTestApp(video.Video{})
	rec := &recordingChat{}
	app.Chat = rec
	app.OBS = &recordingOBS{err: errors.New("obs unreachable")}

	app.carSoundCmd(context.Background(), newTestUser("viewer1"), []string{"highway"})

	if app.carSoundIdx != 0 {
		t.Errorf("a failed switch must not move the current index, got %d", app.carSoundIdx)
	}
	if len(rec.Says) != 1 || !strings.Contains(rec.Says[0], "Couldn't switch") {
		t.Errorf("expected a failure reply, got %v", rec.Says)
	}
}

func TestCarSoundCmd_ListShowsAllNames(t *testing.T) {
	app := newTestApp(video.Video{})
	rec := &recordingChat{}
	app.Chat = rec
	app.OBS = &recordingOBS{}

	app.carSoundCmd(context.Background(), newTestUser("viewer1"), []string{"list"})

	if len(rec.Says) != 1 {
		t.Fatalf("expected one reply, got %v", rec.Says)
	}
	for _, cs := range carSounds {
		if !strings.Contains(rec.Says[0], cs.Name) {
			t.Errorf("list output %q missing sound %q", rec.Says[0], cs.Name)
		}
	}
}

func TestCarSound_PlatformGating(t *testing.T) {
	// Drive the real registered command (with its Platforms scope) through
	// dispatch on each platform, rather than a hand-built Command — the gating
	// lives in the Command.Platforms field set in the registry.
	twitch := &App{Platform: platformTwitch}
	twitch.indexCommands()
	if cmd, _ := twitch.findCommand("!carsound"); cmd != nil {
		t.Error("!carsound must be unavailable on Twitch (the Car Hum source is YouTube-only)")
	}
	if cmd, _ := twitch.findCommand("!carhum"); cmd != nil {
		t.Error("!carhum alias must also be unavailable on Twitch")
	}

	yt := &App{Platform: platformYouTube}
	yt.indexCommands()
	if cmd, _ := yt.findCommand("!carsound"); cmd == nil {
		t.Error("!carsound must be available on YouTube")
	}
	if cmd, _ := yt.findCommand("!carhum"); cmd == nil {
		t.Error("!carhum alias must be available on YouTube")
	}

	// The empty/default platform behaves as Twitch — the one centralized
	// "unset == twitch" assumption.
	dflt := &App{}
	dflt.indexCommands()
	if cmd, _ := dflt.findCommand("!carsound"); cmd != nil {
		t.Error("!carsound must be unavailable on the default (Twitch) platform")
	}
}
