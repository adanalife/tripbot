package chatbot

import (
	"context"
	"errors"
	"slices"
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
	albums   []string // SetAlbum calls, in order
	album    string
	onShare  []string // what Albums() reports; nil means the fan album alone
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

func (f *fakeBeds) Album() string { return f.album }

func (f *fakeBeds) Albums() []string {
	if f.onShare == nil {
		return []string{"fifty-horizons"}
	}
	return f.onShare
}

// ResolveAlbum mirrors the real store's rule (exact name, else unique trailing
// segment) so the command's behavior is exercised against the same contract.
func (f *fakeBeds) ResolveAlbum(arg string) string {
	if arg == "" {
		return ""
	}
	albums := f.Albums()
	if slices.Contains(albums, arg) {
		return arg
	}
	var match string
	for _, a := range albums {
		if strings.HasSuffix(a, "-"+arg) {
			if match != "" {
				return ""
			}
			match = a
		}
	}
	return match
}

func (f *fakeBeds) SetAlbum(_ context.Context, album string) error {
	f.albums = append(f.albums, album)
	if f.err != nil {
		return f.err
	}
	f.bed, f.track, f.album = beds.Album, testTrack, album
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

// An album on the share is accepted in the same argument slot as a bed name or a
// station id — the third namespace the one argument covers.
func TestAudioCmd_AdminPicksAnAlbumByName(t *testing.T) {
	app, fake, out := newAudioTestApp(t, beds.CarHum, "")
	fake.onShare = []string{"fifty-horizons", "lofi-secluded"}

	app.audioCmd(context.Background(), newTestUser(adminUser), []string{"Lofi-Secluded"}) // case-insensitive

	if len(fake.sets) != 0 {
		t.Errorf("an album is a selection, not a bare bed switch, got %v", fake.sets)
	}
	if len(fake.albums) != 1 || fake.albums[0] != "lofi-secluded" {
		t.Fatalf("expected one selection of lofi-secluded, got %v", fake.albums)
	}
	if got := out(); !strings.Contains(got, "Lofi Secluded") {
		t.Errorf("expected the announcement to name the album, got %q", got)
	}
}

// Typing the genre prefix is what the shorthand exists to avoid: the share wants
// "synthwave-rose" so albums of a genre sort together, chat wants "rose".
func TestAudioCmd_AdminPicksAnAlbumByShorthand(t *testing.T) {
	app, fake, out := newAudioTestApp(t, beds.CarHum, "")
	fake.onShare = []string{"fifty-horizons", "synthwave-rose", "lofi-secluded"}

	app.audioCmd(context.Background(), newTestUser(adminUser), []string{"rose"})

	if len(fake.albums) != 1 || fake.albums[0] != "synthwave-rose" {
		t.Fatalf("expected the shorthand to reach synthwave-rose, got %v", fake.albums)
	}
	if got := out(); !strings.Contains(got, "Synthwave Rose") {
		t.Errorf("expected the announcement to name the album, got %q", got)
	}
}

// The fan album carries its credit rather than its directory name, since the
// directory is not what anyone would call it.
func TestAudioCmd_AlbumAnnouncementUsesTheCreditNotTheDirectory(t *testing.T) {
	app, _, out := newAudioTestApp(t, beds.CarHum, "")

	app.audioCmd(context.Background(), newTestUser(adminUser), []string{"fifty-horizons"})

	got := out()
	if !strings.Contains(got, "Fifty Horizons, by wooderCZ") {
		t.Errorf("expected the fan album's credit, got %q", got)
	}
	if strings.Contains(got, "fifty-horizons") {
		t.Errorf("expected the credit rather than the directory name, got %q", got)
	}
}

// An album with no credit in albumDescs is still playable, and announces as its
// directory read aloud rather than as a slug. A missing credit must not make an
// album unreachable — the share grows faster than the table.
func TestAudioCmd_UncreditedAlbumIsNamedFromItsDirectory(t *testing.T) {
	app, fake, out := newAudioTestApp(t, beds.CarHum, "")
	fake.onShare = []string{"synthwave-lone-wolf"}

	app.audioCmd(context.Background(), newTestUser(adminUser), []string{"lone-wolf"})

	if len(fake.albums) != 1 || fake.albums[0] != "synthwave-lone-wolf" {
		t.Fatalf("expected the uncredited album to be selected, got %v", fake.albums)
	}
	got := out()
	if !strings.Contains(got, "Synthwave Lone Wolf") {
		t.Errorf("expected the directory read aloud, got %q", got)
	}
	if strings.Contains(got, "synthwave-lone-wolf") {
		t.Errorf("expected no raw slug in chat, got %q", got)
	}
}

func TestAlbumName(t *testing.T) {
	for _, tc := range []struct{ album, want string }{
		{"fifty-horizons", "Fifty Horizons, by wooderCZ"}, // credited
		{"synthwave-rose", "Synthwave Rose"},
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

func TestAudioCmd_UnknownBedListsOptionsWithoutSwitching(t *testing.T) {
	app, fake, out := newAudioTestApp(t, beds.CarHum, "")
	fake.onShare = []string{"fifty-horizons", "lofi-secluded"}

	app.audioCmd(context.Background(), newTestUser(adminUser), []string{"spaceship"})

	if len(fake.sets) != 0 {
		t.Fatalf("an unknown bed must not switch, got %v", fake.sets)
	}
	if len(fake.albums) != 0 {
		t.Fatalf("an unknown name must not select an album, got %v", fake.albums)
	}
	got := out()
	for _, b := range beds.All {
		if !strings.Contains(got, string(b)) {
			t.Errorf("expected %q in the options list, got %q", b, got)
		}
	}
	// The albums are ours and few, so they're named outright rather than linked.
	for _, a := range fake.onShare {
		if !strings.Contains(got, a) {
			t.Errorf("expected album %q in the options list, got %q", a, got)
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
