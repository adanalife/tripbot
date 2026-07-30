package chatbot

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/adanalife/tripbot/pkg/obs/beds"
	"github.com/adanalife/tripbot/pkg/video"
)

// --- App.OBS seam fakes ---

// noopOBS swallows every OBS call — the default in newTestApp.
type noopOBS struct{}

func (noopOBS) RefreshBrowserSources(context.Context) (int, error) { return 0, nil }

// recordingOBS returns a primed browser-source refresh count, or an error.
type recordingOBS struct {
	Refreshed  int
	refreshErr error
}

func (r *recordingOBS) RefreshBrowserSources(context.Context) (int, error) {
	return r.Refreshed, r.refreshErr
}

// fakeBeds stands in for *beds.Store: it records every Set so tests can assert
// a switch happened (or didn't), and mirrors the store's "only the album has a
// track" shape.
type fakeBeds struct {
	bed      beds.Bed
	track    string
	station  string
	artist   string
	title    string
	feedErr  error
	sets     []beds.Bed
	stations []string
	feeds    int
	err      error
}

const testTrack = "/opt/tripbot/assets/music/fifty-horizons/Colorado Sunrise.mp3"

func (f *fakeBeds) Current() (beds.Bed, string) { return f.bed, f.track }

func (f *fakeBeds) SomaFMTrack(context.Context) (string, string, error) {
	f.feeds++
	return f.artist, f.title, f.feedErr
}

func (f *fakeBeds) Station() string {
	if f.station == "" {
		return beds.DefaultStation
	}
	return f.station
}

func (f *fakeBeds) Set(_ context.Context, bed beds.Bed) error {
	f.sets = append(f.sets, bed)
	if f.err != nil {
		return f.err
	}
	f.bed, f.track = bed, ""
	if bed == beds.Album {
		f.track = testTrack
	}
	return nil
}

func (f *fakeBeds) SetStation(_ context.Context, station string) error {
	f.stations = append(f.stations, station)
	if f.err != nil {
		return f.err
	}
	f.bed, f.track, f.station = beds.SomaFM, "", station
	return nil
}

// newAudioTestApp wires an App with a fake bed store on the given bed. adminUser
// passes UserIsAdmin (it matches testConf.ChannelName); any other name is a
// plain viewer.
func newAudioTestApp(t *testing.T, bed beds.Bed, track string) (*App, *fakeBeds, func() string) {
	t.Helper()
	app := newTestApp(video.Video{})
	fake := &fakeBeds{bed: bed, track: track}
	app.Beds = fake
	return app, fake, captureSay(t, app)
}

func TestAudioCmd_ViewerGetsReportWithoutSwitching(t *testing.T) {
	app, fake, out := newAudioTestApp(t, beds.CarHum, "")

	app.audioCmd(context.Background(), newTestUser("viewer1"), nil)

	if len(fake.sets) != 0 {
		t.Errorf("no-arg !audio must not switch, got %v", fake.sets)
	}
	if got := out(); !strings.Contains(got, bedDescs[beds.CarHum]) {
		t.Errorf("expected the report to name the live bed, got %q", got)
	}
}

func TestAudioCmd_ViewerNamingABedStillCannotSwitch(t *testing.T) {
	app, fake, out := newAudioTestApp(t, beds.CarHum, "")

	app.audioCmd(context.Background(), newTestUser("viewer1"), []string{"somafm"})

	if len(fake.sets) != 0 {
		t.Fatalf("a non-admin must not switch the bed, got %v", fake.sets)
	}
	if got := out(); !strings.Contains(got, bedDescs[beds.CarHum]) {
		t.Errorf("a refused switch should still report what's playing, got %q", got)
	}
}

func TestAudioCmd_AdminSwitchesAndAnnouncesTheTrack(t *testing.T) {
	app, fake, out := newAudioTestApp(t, beds.CarHum, "")

	app.audioCmd(context.Background(), newTestUser(adminUser), []string{"Album"}) // case-insensitive

	if len(fake.sets) != 1 || fake.sets[0] != beds.Album {
		t.Fatalf("expected one switch to the album, got %v", fake.sets)
	}
	got := out()
	if !strings.Contains(got, bedDescs[beds.Album]) {
		t.Errorf("expected the announcement to name the album, got %q", got)
	}
	if !strings.Contains(got, "Colorado Sunrise") {
		t.Errorf("expected the announcement to name the playing track, got %q", got)
	}
}

// A SomaFM channel id is accepted in the same argument slot as a bed name — no
// channel id collides with one, and "switch the music to X" is one intent.
func TestAudioCmd_AdminTunesASomaFMStation(t *testing.T) {
	app, fake, out := newAudioTestApp(t, beds.CarHum, "")

	app.audioCmd(context.Background(), newTestUser(adminUser), []string{"DroneZone"}) // case-insensitive

	if len(fake.sets) != 0 {
		t.Errorf("a station is a tune, not a bed switch, got %v", fake.sets)
	}
	if len(fake.stations) != 1 || fake.stations[0] != "dronezone" {
		t.Fatalf("expected one tune to dronezone, got %v", fake.stations)
	}
	if got := out(); !strings.Contains(got, "Drone Zone") {
		t.Errorf("expected the announcement to name the channel, got %q", got)
	}
}

func TestAudioCmd_UnknownBedListsOptionsWithoutSwitching(t *testing.T) {
	app, fake, out := newAudioTestApp(t, beds.CarHum, "")

	app.audioCmd(context.Background(), newTestUser(adminUser), []string{"spaceship"})

	if len(fake.sets) != 0 {
		t.Fatalf("an unknown bed must not switch, got %v", fake.sets)
	}
	got := out()
	for _, b := range beds.All {
		if !strings.Contains(got, string(b)) {
			t.Errorf("expected %q in the options list, got %q", b, got)
		}
	}
}

func TestAudioCmd_FailedSwitchDoesNotClaimSuccess(t *testing.T) {
	app, fake, out := newAudioTestApp(t, beds.CarHum, "")
	fake.err = errors.New("no album tracks mounted")

	app.audioCmd(context.Background(), newTestUser(adminUser), []string{"album"})

	if got := out(); strings.Contains(got, "Switched") {
		t.Errorf("a rejected switch must not announce success, got %q", got)
	}
}

func TestAudioCmd_NoBedStoreReportsUnavailable(t *testing.T) {
	app := newTestApp(video.Video{})
	app.Beds = nil
	out := captureSay(t, app)

	app.audioCmd(context.Background(), newTestUser(adminUser), []string{"album"})

	if got := out(); got == "" {
		t.Error("expected an unavailable message rather than silence")
	}
}

func TestAudio_AvailableOnEveryPlatform(t *testing.T) {
	// Every platform's OBS scene ships the one Background Audio source, so
	// unlike the car-hum-variant command this replaced, !audio isn't scoped.
	for _, platform := range knownPlatforms() {
		app := newTestApp(video.Video{})
		app.Platform = platform
		app.indexCommands()
		if cmd, _ := app.findCommand("!audio"); cmd == nil {
			t.Errorf("!audio must be available on %s", platform)
		}
	}
}
