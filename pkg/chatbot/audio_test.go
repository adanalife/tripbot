package chatbot

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/adanalife/tripbot/pkg/obs/beds"
	"github.com/adanalife/tripbot/pkg/video"
)

// The radio bed's station list is mounted at runtime; the tests tune this one.
func init() {
	beds.Stations = []beds.Station{{ID: "lofi", Name: "Example Lo-Fi", URL: "https://radio.example/lofi"}}
}

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
	pending  *beds.Switch // a switch waiting out the delay; nil = none
	feeds    int
	err      error
	// the tuned station's library and what !request queued from it
	tracks     []string
	tracksErr  error
	trackLists int
	pushed     []string
}

func (f *fakeBeds) Tracks(context.Context) ([]string, error) {
	f.trackLists++
	return f.tracks, f.tracksErr
}

func (f *fakeBeds) Push(_ context.Context, file string) error {
	f.pushed = append(f.pushed, file)
	return nil
}

const testTrack = "/opt/tripbot/assets/music/fifty-horizons/Colorado Sunrise.mp3"

func (f *fakeBeds) Playing() (beds.Bed, string) { return f.bed, f.track }

func (f *fakeBeds) RadioTrack(context.Context) (string, string, error) {
	f.feeds++
	return f.artist, f.title, f.feedErr
}

// Pending models the real store's switch delay: nil is a store applying switches
// inline (the tests below drive the effects directly), and a test that cares
// about the waiting state sets it.
func (f *fakeBeds) Pending() (beds.Switch, bool) {
	if f.pending == nil {
		return beds.Switch{}, false
	}
	return *f.pending, true
}

func (f *fakeBeds) Station() string {
	if f.station == "" {
		return beds.DefaultStation()
	}
	return f.station
}

func (f *fakeBeds) Set(_ context.Context, bed beds.Bed) error {
	f.sets = append(f.sets, bed)
	if f.err != nil {
		return f.err
	}
	f.bed, f.track = bed, ""
	return nil
}

func (f *fakeBeds) SetStation(_ context.Context, station string) error {
	f.stations = append(f.stations, station)
	if f.err != nil {
		return f.err
	}
	f.bed, f.track, f.station = beds.Radio, "", station
	return nil
}

// PlayingAlbum is what the track path says, which is what the real store derives
// it from.
func (f *fakeBeds) PlayingAlbum() string {
	if f.bed != beds.Album || f.track == "" {
		return ""
	}
	return filepath.Base(filepath.Dir(f.track))
}

// newAudioTestApp wires an App with a fake bed store on the given bed.
// newAdminUser() passes the admin gate (its Broadcaster flag); any plain
// newTestUser is a viewer.
func newAudioTestApp(t *testing.T, bed beds.Bed, track string) (*App, *fakeBeds, func() string) {
	t.Helper()
	app := newTestApp(video.Video{})
	fake := &fakeBeds{bed: bed, track: track}
	app.Beds = fake
	out, says := captureSay(t, app)
	// Every command exercised through this app answers with a single chat
	// line, so the count is asserted here rather than in all sixteen tests.
	t.Cleanup(func() {
		if says() != 1 {
			t.Errorf("expected exactly one Say() call, got %d", says())
		}
	})
	return app, fake, out
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

	app.audioCmd(context.Background(), newTestUser("viewer1"), []string{"radio"})

	if len(fake.sets) != 0 {
		t.Fatalf("a non-admin must not switch the bed, got %v", fake.sets)
	}
	if got := out(); !strings.Contains(got, bedDescs[beds.CarHum]) {
		t.Errorf("a refused switch should still report what's playing, got %q", got)
	}
}

// A radio station id is accepted in the same argument slot as a bed name — no
// channel id collides with one, and "switch the music to X" is one intent.
func TestAudioCmd_AdminTunesARadioStation(t *testing.T) {
	app, fake, out := newAudioTestApp(t, beds.CarHum, "")

	app.audioCmd(context.Background(), newAdminUser(), []string{"LoFi"}) // case-insensitive

	if len(fake.sets) != 0 {
		t.Errorf("a station is a tune, not a bed switch, got %v", fake.sets)
	}
	if len(fake.stations) != 1 || fake.stations[0] != "lofi" {
		t.Fatalf("expected one tune to lofi, got %v", fake.stations)
	}
	if got := out(); !strings.Contains(got, "Example Lo-Fi") {
		t.Errorf("expected the announcement to name the channel, got %q", got)
	}
}

func TestAlbumName(t *testing.T) {
	for _, tc := range []struct{ album, want string }{
		{"fifty-horizons", "Fifty Horizons, by wooderCZ"}, // credited
		{"streambeats-synthwave-rose", "Streambeats Synthwave Rose"},
		{"lofi-certain-shade-of-blue", "Lofi Certain Shade Of Blue"},
		{"lofi-in-4k", "Lofi In 4k"},
		{"diamonds", "Diamonds"}, // no prefix
		{"", ""},
	} {
		if got := albumName(tc.album); got != tc.want {
			t.Errorf("albumName(%q): want %q, got %q", tc.album, tc.want, got)
		}
	}
}

// "album" is the radio's fallback, not a bed, so it lands with the other
// unknown names: no switch, and the real options listed.
func TestAudioCmd_UnknownBedListsOptionsWithoutSwitching(t *testing.T) {
	for _, arg := range []string{"spaceship", "album"} {
		t.Run(arg, func(t *testing.T) {
			app, fake, out := newAudioTestApp(t, beds.CarHum, "")

			app.audioCmd(context.Background(), newAdminUser(), []string{arg})

			if len(fake.sets) != 0 || len(fake.stations) != 0 {
				t.Fatalf("an unknown name must not switch, got %v %v", fake.sets, fake.stations)
			}
			got := out()
			if want := fmt.Sprintf("🎵 No background audio called %q. Options: radio, carhum, lofi", arg); got != want {
				t.Errorf("got %q, want %q", got, want)
			}
		})
	}
}

func TestAudioCmd_FailedSwitchDoesNotClaimSuccess(t *testing.T) {
	app, fake, out := newAudioTestApp(t, beds.CarHum, "")
	fake.err = errors.New("obs unreachable")

	app.audioCmd(context.Background(), newAdminUser(), []string{"carhum"})

	if got := out(); strings.Contains(got, "Switched") {
		t.Errorf("a rejected switch must not announce success, got %q", got)
	}
}

func TestAudioCmd_NoBedStoreReportsUnavailable(t *testing.T) {
	app := newTestApp(video.Video{})
	app.Beds = nil
	out, says := captureSay(t, app)

	app.audioCmd(context.Background(), newAdminUser(), []string{"album"})

	if got := out(); got == "" {
		t.Error("expected an unavailable message rather than silence")
	}
	if says() != 1 {
		t.Errorf("expected exactly one Say() call, got %d", says())
	}
}

func TestAudio_AvailableOnEveryPlatform(t *testing.T) {
	// Every platform's OBS scene ships the one Background Audio source, so
	// unlike the car-hum-variant command this replaced, !audio isn't scoped.
	for _, platform := range knownPlatforms() {
		app := newTestApp(video.Video{})
		app.Platform = platform
		app.indexCommands()
		if cmd, _, _ := app.findCommand("!audio"); cmd == nil {
			t.Errorf("!audio must be available on %s", platform)
		}
	}
}

// The fallback spans many albums, so chat names the one the track sits in.
func TestSongCmd_NamesTheFallbackAlbumOnAir(t *testing.T) {
	app, fake, out := newAudioTestApp(t, beds.Album, "")
	fake.track = "/opt/tripbot/assets/music/streambeats-synthwave-rose/07 Petals.mp3"

	app.songCmd(context.Background(), newTestUser("viewer1"), nil)

	got := out()
	if !strings.Contains(got, "Streambeats Synthwave Rose") {
		t.Errorf("expected the album the track sits in, got %q", got)
	}
	if !strings.Contains(got, "Petals") {
		t.Errorf("expected the track title, got %q", got)
	}
}

// Every file the StreamBeats albums ship carries the label and the album in its
// own name, which the directory carries too — announced whole, one line says
// "StreamBeats" twice, "Breaker" twice, and a track number.
func TestSongCmd_SaysTheAlbumAndLabelOnce(t *testing.T) {
	app, fake, out := newAudioTestApp(t, beds.Album, "")
	fake.track = "/opt/tripbot/assets/music/streambeats-synthwave-breaker/" +
		"StreamBeats by Harris Heller - Breaker - 21 Holosmith.flac"

	app.songCmd(context.Background(), newTestUser("viewer1"), nil)

	got := out()
	if want := `♪ Now playing: "Holosmith" — Breaker, StreamBeats by Harris Heller`; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// A switch waits before it lands, so the announcement is future tense and says
// how long — that wait is the window a second !audio can correct it in, which
// nobody can use without being told it exists.
func TestAudioCmd_AnnouncesAPendingSwitchInTheFuture(t *testing.T) {
	app, fake, out := newAudioTestApp(t, beds.CarHum, "")
	fake.pending = &beds.Switch{
		Bed:     beds.Radio,
		Station: "lofi",
		At:      time.Now().Add(5 * time.Second),
	}

	app.audioCmd(context.Background(), newAdminUser(), []string{"lofi"})

	if got := out(); got != "🎵 Switching to Example Lo-Fi on the radio in 5s" {
		t.Errorf("got %q", got)
	}
}

// A private station is neither selectable from chat nor offered as an option.
func TestAudioCmd_PrivateStationIsNotAnOption(t *testing.T) {
	prev := beds.Stations
	beds.Stations = append([]beds.Station{{ID: "home", Name: "Home", URL: "https://radio.example/home", Private: true}}, prev...)
	t.Cleanup(func() { beds.Stations = prev })

	app, fake, out := newAudioTestApp(t, beds.CarHum, "")
	app.audioCmd(context.Background(), newAdminUser(), []string{"home"})

	if len(fake.stations) != 0 {
		t.Fatalf("a private station was tuned: %v", fake.stations)
	}
	got := out()
	if !strings.Contains(got, "lofi") || strings.Contains(got, "home,") || strings.HasSuffix(got, "home") {
		t.Errorf("options should list lofi and not the private station, got %q", got)
	}
}
