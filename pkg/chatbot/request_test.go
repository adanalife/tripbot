package chatbot

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/adanalife/tripbot/pkg/obs/beds"
)

func TestMatchTrack(t *testing.T) {
	files := []string{
		"/music/Fifty Horizons/Colorado Sunrise.mp3",
		"/music/Fifty Horizons/Sunrise Over Denver (Live).mp3",
		"/music/Night Drive/Neon Rain.flac",
	}
	for _, tc := range []struct {
		query, want string
		ok          bool
	}{
		{"neon rain", "/music/Night Drive/Neon Rain.flac", true},
		{"SUNRISE", "/music/Fifty Horizons/Colorado Sunrise.mp3", true}, // shortest name wins the tie
		{"sunrise live", "/music/Fifty Horizons/Sunrise Over Denver (Live).mp3", true},
		{"fifty horizons denver", "/music/Fifty Horizons/Sunrise Over Denver (Live).mp3", true},
		{"rain neon", "/music/Night Drive/Neon Rain.flac", true},
		{"moonlight", "", false},
	} {
		got, ok := matchTrack(files, tc.query)
		if got != tc.want || ok != tc.ok {
			t.Errorf("matchTrack(%q) = (%q, %v), want (%q, %v)", tc.query, got, ok, tc.want, tc.ok)
		}
	}
}

func TestRequestCmd_QueuesTheMatchedTrack(t *testing.T) {
	app, fake, out := newAudioTestApp(t, beds.Radio, "")
	fake.tracks = []string{"/music/Night Drive/Neon Rain.flac", "/music/a/Other.mp3"}

	app.requestCmd(context.Background(), newAdminUser(), []string{"neon", "rain"})

	if len(fake.pushed) != 1 || fake.pushed[0] != "/music/Night Drive/Neon Rain.flac" {
		t.Errorf("pushed %v, want the matched file", fake.pushed)
	}
	if got := out(); !strings.Contains(got, `"Neon Rain"`) || !strings.Contains(got, "play next") {
		t.Errorf("reply = %q, want the queued track by name", got)
	}
}

func TestRequestCmd_NoMatchPushesNothing(t *testing.T) {
	app, fake, out := newAudioTestApp(t, beds.Radio, "")
	fake.tracks = []string{"/music/a/Other.mp3"}

	app.requestCmd(context.Background(), newAdminUser(), []string{"moonlight"})

	if len(fake.pushed) != 0 {
		t.Errorf("pushed %v, want nothing", fake.pushed)
	}
	if got := out(); !strings.Contains(got, "Nothing on") {
		t.Errorf("reply = %q, want a miss", got)
	}
}

func TestRequestCmd_ThirdPartyStation(t *testing.T) {
	app, fake, out := newAudioTestApp(t, beds.Radio, "")
	fake.tracksErr = beds.ErrNoRequests

	app.requestCmd(context.Background(), newAdminUser(), []string{"anything"})

	if len(fake.pushed) != 0 {
		t.Errorf("pushed %v, want nothing", fake.pushed)
	}
	if got := out(); !strings.Contains(got, "doesn't take requests") {
		t.Errorf("reply = %q", got)
	}
}

func TestRequestCmd_UsageWithoutASong(t *testing.T) {
	app, fake, out := newAudioTestApp(t, beds.Radio, "")

	app.requestCmd(context.Background(), newAdminUser(), nil)

	if len(fake.pushed) != 0 || fake.trackLists != 0 {
		t.Errorf("no song should touch the radio: pushed %v, listed %d", fake.pushed, fake.trackLists)
	}
	if got := out(); !strings.Contains(got, "Usage") {
		t.Errorf("reply = %q", got)
	}
}

func TestRequestCmd_RadioDown(t *testing.T) {
	app, fake, out := newAudioTestApp(t, beds.Radio, "")
	fake.tracksErr = errors.New("connection refused")

	app.requestCmd(context.Background(), newAdminUser(), []string{"x"})

	if got := out(); !strings.Contains(got, "Couldn't reach the radio") {
		t.Errorf("reply = %q", got)
	}
}
