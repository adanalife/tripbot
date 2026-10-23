package beds

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

// TestMain switches off the advance debounce, which exists to drop a second
// report of one track ending (see Advance) and would otherwise swallow the
// back-to-back advances these tests drive by hand. The guard itself is covered
// by TestAdvance_DropsADuplicateEndingReport, which turns it back on.
// The switch delay goes off for the same reason: these tests read OBS straight
// after a Set, and zero applies inline. TestSchedule_* turn it back on.
// testStations stands in for the mounted station list; the first is the default.
var testStations = []Station{
	{ID: "ambient", Name: "Example Ambient", URL: "https://radio.example/ambient"},
	{ID: "lofi", Name: "Example Lo-Fi", URL: "https://radio.example/lofi"},
	{ID: "jazz", Name: "Example Jazz", URL: "https://radio.example/jazz.mp3"},
}

func TestMain(m *testing.M) {
	advanceDebounce = 0
	switchDelay = 0
	Stations = testStations
	os.Exit(m.Run())
}

// fakeOBS records what would have been written to the OBS source.
type fakeOBS struct {
	network  bool
	url      string
	file     string
	loop     bool
	settings map[string]any
	err      error
}

func (f *fakeOBS) SetNetwork(_ context.Context, _, url string) error {
	if f.err != nil {
		return f.err
	}
	f.network, f.url, f.file = true, url, ""
	return nil
}

func (f *fakeOBS) SetLocalFile(_ context.Context, _, file string, loop bool) error {
	if f.err != nil {
		return f.err
	}
	f.network, f.file, f.loop = false, file, loop
	return nil
}

func (f *fakeOBS) Settings(context.Context, string) (map[string]any, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.settings, nil
}

// shareDir builds a share holding one album of n tracks. It mirrors the real
// layout: the album is a SUBDIRECTORY, and the share root also holds a loose
// audio file (the 556MB carsounds.m4a lives there) plus a non-audio file —
// neither of which is a track. Returns the share root, which is what gets
// mounted. shareOf builds the multi-album case.
func shareDir(t *testing.T, n int) string {
	t.Helper()
	dir := t.TempDir()
	album := filepath.Join(dir, "fifty-horizons")
	if err := os.MkdirAll(album, 0o700); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < n; i++ {
		name := filepath.Join(album, string(rune('a'+i))+" track.mp3")
		if err := os.WriteFile(name, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, loose := range []string{"carsounds.m4a", "readme.txt"} {
		if err := os.WriteFile(filepath.Join(dir, loose), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// shareOf builds a share of several albums, each with the given track count —
// the layout once StreamBeats lands beside the fan album. Carries the same loose
// root files as shareDir, since those must stay out of every album.
func shareOf(t *testing.T, albums map[string]int) string {
	t.Helper()
	dir := t.TempDir()
	for album, n := range albums {
		if err := os.MkdirAll(filepath.Join(dir, album), 0o700); err != nil {
			t.Fatal(err)
		}
		for i := 0; i < n; i++ {
			name := filepath.Join(dir, album, string(rune('a'+i))+" track.mp3")
			if err := os.WriteFile(name, nil, 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, loose := range []string{"carsounds.m4a", "readme.txt"} {
		if err := os.WriteFile(filepath.Join(dir, loose), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// realShare mirrors the live layout: the fan album plus StreamBeats albums named
// streambeats-<genre>-<album>, which is what makes "streambeats" and
// "streambeats-lofi" groups. Carries an empty directory that must not read as an
// album.
func realShare(t *testing.T) string {
	t.Helper()
	dir := shareOf(t, map[string]int{
		"fifty-horizons":                3,
		"streambeats-ambient-diamonds":  2,
		"streambeats-lofi-secluded":     4,
		"streambeats-lofi-gold":         2,
		"streambeats-synthwave-rose":    2,
		"streambeats-synthwave-breaker": 3,
	})
	if err := os.MkdirAll(filepath.Join(dir, "streambeats-lofi-empty"), 0o700); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestPlayingAlbum_EmptyOffTheAlbumBed(t *testing.T) {
	s := NewStore(&fakeOBS{}, CarHum, realShare(t), "twitch")
	if got := s.PlayingAlbum(); got != "" {
		t.Errorf("car hum has no album, got %q", got)
	}
}

func TestAlbumFromFile(t *testing.T) {
	const share = "/opt/tripbot/assets/music"
	for _, tc := range []struct{ file, want string }{
		{share + "/streambeats-lofi-secluded/a.mp3", "streambeats-lofi-secluded"},
		// The whole directory name is the album — the group prefixes inside it are
		// a naming convention, not a hierarchy on disk.
		{share + "/fifty-horizons/deep/b.mp3", "fifty-horizons"}, // nesting belongs to the top album
		{share + "/carsounds.m4a", ""},                           // loose at the root, no album
		{"/opt/tripbot/assets/carhum/car-hum-idle.flac", ""},     // another bed entirely
		{"", ""},
	} {
		if got := albumFromFile(tc.file, share); got != tc.want {
			t.Errorf("albumFromFile(%q): want %q, got %q", tc.file, tc.want, got)
		}
	}
}

func TestSet_RadioUsesNetworkMode(t *testing.T) {
	o := &fakeOBS{}
	s := NewStore(o, CarHum, shareDir(t, 2), "twitch")
	if err := s.Set(context.Background(), Radio); err != nil {
		t.Fatal(err)
	}
	if !o.network {
		t.Fatal("radio should put the source in network mode")
	}
	if bed, track := s.Current(); bed != Radio || track != "" {
		t.Fatalf("current: want radio/no track, got %s/%s", bed, track)
	}
}

// The station is what makes the radio bed a choice rather than one stream, so
// tuning has to reach OBS as a URL and select the bed in one move — nobody means
// "tune a station I'm not playing".
func TestSetStation_TunesAndSelectsTheRadioBed(t *testing.T) {
	o := &fakeOBS{}
	s := NewStore(o, CarHum, shareDir(t, 2), "twitch")
	if err := s.SetStation(context.Background(), "lofi"); err != nil {
		t.Fatal(err)
	}
	if o.url != StreamURL("lofi") {
		t.Fatalf("obs url = %q, want %q", o.url, StreamURL("lofi"))
	}
	if bed, _ := s.Current(); bed != Radio {
		t.Fatalf("bed = %s, want radio", bed)
	}
	if s.Station() != "lofi" {
		t.Fatalf("station = %q, want lofi", s.Station())
	}
}

// A station OBS wouldn't accept must not become the one we report: !song and the
// console's now-playing line would then name a channel nobody is hearing.
func TestSetStation_RollsBackWhenOBSRejectsIt(t *testing.T) {
	o := &fakeOBS{err: errors.New("obs unreachable")}
	s := NewStore(o, Radio, t.TempDir(), "twitch")
	if err := s.SetStation(context.Background(), "lofi"); err == nil {
		t.Fatal("want an error when OBS rejects the tune")
	}
	if s.Station() != DefaultStation() {
		t.Fatalf("station = %q, want the previous %q", s.Station(), DefaultStation())
	}
}

func TestSetStation_RejectsAnUnknownChannel(t *testing.T) {
	s := NewStore(&fakeOBS{}, Radio, t.TempDir(), "twitch")
	if err := s.SetStation(context.Background(), "not-a-channel"); err == nil {
		t.Fatal("want an error for a station the list doesn't have")
	}
}

// Switching away and back has to land on the tuned station, not the default:
// the station outlives the bed.
func TestSet_RadioReturnsToTheTunedStation(t *testing.T) {
	o := &fakeOBS{}
	s := NewStore(o, Radio, shareDir(t, 2), "twitch")
	if err := s.SetStation(context.Background(), "jazz"); err != nil {
		t.Fatal(err)
	}
	if err := s.Set(context.Background(), CarHum); err != nil {
		t.Fatal(err)
	}
	if err := s.Set(context.Background(), Radio); err != nil {
		t.Fatal(err)
	}
	if o.url != StreamURL("jazz") {
		t.Fatalf("obs url = %q, want the tuned %q", o.url, StreamURL("jazz"))
	}
}

func TestStationFromURL(t *testing.T) {
	for _, tc := range []struct{ url, want string }{
		{StreamURL("lofi"), "lofi"},
		{"https://radio.example/jazz.mp3", "jazz"},
		// Only an exact match is a station: a sibling path, another host, or a
		// URL that merely ends in a station id is not.
		{"https://radio.example/lofi-256", ""},
		{"https://elsewhere.example/lofi", ""},
		{"rtmp://example.com/lofi", ""},
		{"", ""},
	} {
		if got := stationFromURL(tc.url); got != tc.want {
			t.Errorf("stationFromURL(%q) = %q, want %q", tc.url, got, tc.want)
		}
	}
}

func TestSet_CarHumLoops(t *testing.T) {
	o := &fakeOBS{}
	s := NewStore(o, Radio, shareDir(t, 2), "twitch")
	if err := s.Set(context.Background(), CarHum); err != nil {
		t.Fatal(err)
	}
	if o.file != CarHumFile(DefaultVoicing) {
		t.Fatalf("carhum file: want %s, got %s", CarHumFile(DefaultVoicing), o.file)
	}
	// A drone that stops leaves dead air — it has to loop.
	if !o.loop {
		t.Fatal("carhum must loop")
	}
}

func TestFallback_IgnoresLooseFilesAtTheShareRoot(t *testing.T) {
	// The real share keeps carsounds.m4a — 556MB, nine hours — beside the album
	// directories. Treating it as a track would put it on the stream and stall
	// the rotation until it ended.
	dir := shareDir(t, 3)
	o := &fakeOBS{}
	s := NewStore(o, Radio, dir, "twitch")
	for i := 0; i < 6; i++ {
		if err := s.SwapToFallback(context.Background()); err != nil {
			t.Fatal(err)
		}
		if filepath.Base(o.file) == "carsounds.m4a" {
			t.Fatal("picked the loose carsounds.m4a from the share root")
		}
		if err := s.Advance(context.Background()); err != nil {
			t.Fatal(err)
		}
		if filepath.Base(o.file) == "carsounds.m4a" {
			t.Fatal("advanced onto the loose carsounds.m4a from the share root")
		}
	}
}

func TestSet_FailedSwitchKeepsReportingTheOldBed(t *testing.T) {
	o := &fakeOBS{err: errors.New("obs unreachable")}
	s := NewStore(o, Radio, shareDir(t, 2), "twitch")
	if err := s.Set(context.Background(), CarHum); err == nil {
		t.Fatal("want an error when OBS rejects the switch")
	}
	if bed, _ := s.Current(); bed != Radio {
		t.Fatalf("bed after a rejected switch: want radio, got %s", bed)
	}
}

func TestSet_RejectsUnknownBed(t *testing.T) {
	s := NewStore(&fakeOBS{}, CarHum, t.TempDir(), "twitch")
	if err := s.Set(context.Background(), Bed("wurlitzer")); err == nil {
		t.Fatal("want an error for an unknown bed")
	}
}

func TestAdvance_WalksEveryTrackThenWraps(t *testing.T) {
	const n = 4
	o := &fakeOBS{}
	s := NewStore(o, Radio, shareDir(t, n), "twitch")
	if err := s.SwapToFallback(context.Background()); err != nil {
		t.Fatal(err)
	}

	seen := map[string]bool{o.file: true}
	for i := 0; i < n-1; i++ {
		if err := s.Advance(context.Background()); err != nil {
			t.Fatal(err)
		}
		if seen[o.file] {
			t.Fatalf("track %q repeated before the album finished", o.file)
		}
		seen[o.file] = true
	}
	if len(seen) != n {
		t.Fatalf("played %d of %d tracks before wrapping", len(seen), n)
	}
	// One more advance wraps to a fresh shuffle rather than stopping.
	if err := s.Advance(context.Background()); err != nil {
		t.Fatal(err)
	}
	if o.file == "" {
		t.Fatal("advance past the end should wrap, not stop")
	}
}

// One track ending is reported twice — by the playback-ended subscription and
// by the watchdog tick that backs it up — and acting on both would skip a track.
func TestAdvance_DropsADuplicateEndingReport(t *testing.T) {
	defer func(d time.Duration) { advanceDebounce = d }(advanceDebounce)
	advanceDebounce = time.Minute

	o := &fakeOBS{}
	s := NewStore(o, Radio, shareOf(t, map[string]int{"streambeats-lofi-gold": 4}), "twitch")
	ctx := context.Background()
	if err := s.SwapToFallback(ctx); err != nil {
		t.Fatal(err)
	}
	// Age the swap's own stamp: what's under test is two advances against each
	// other, not an advance against the swap that started the album.
	s.mu.Lock()
	s.lastStart = time.Time{}
	s.mu.Unlock()

	for range 2 {
		if err := s.Advance(ctx); err != nil {
			t.Fatal(err)
		}
	}
	// One advance off the first track lands on the second, and the duplicate
	// must not reach the third.
	if want := s.tracks[1]; o.file != want {
		t.Errorf("two reports of one ending played %q, want %q", o.file, want)
	}
}

func TestAdvance_NoopOnOtherBeds(t *testing.T) {
	o := &fakeOBS{}
	s := NewStore(o, CarHum, shareDir(t, 3), "twitch")
	if err := s.Set(context.Background(), CarHum); err != nil {
		t.Fatal(err)
	}
	if err := s.Advance(context.Background()); err != nil {
		t.Fatal(err)
	}
	if o.file != CarHumFile(DefaultVoicing) {
		t.Fatalf("advance moved a non-album bed: %s", o.file)
	}
}

// An album fallback with no play order is the one way the stream goes silent
// while OBS reports a healthy source: OBS boots on an album track, and a missing
// track index leaves Detect nothing to build the order from.
func TestAdvance_EmptyAlbumFallsBackToTheCarHum(t *testing.T) {
	dir := t.TempDir() // a share with no albums: nothing to build an order from
	playing := filepath.Join(dir, "fifty-horizons", "a track.mp3")
	o := &fakeOBS{settings: map[string]any{"is_local_file": true, "local_file": playing}}
	s := NewStore(o, CarHum, dir, "twitch")
	s.Detect(context.Background())

	if bed, _ := s.Playing(); bed != Album {
		t.Fatalf("setup: want the album fallback read off OBS, got %q", bed)
	}
	if err := s.Advance(context.Background()); err != nil {
		t.Fatal(err)
	}

	if o.file != FallbackFile || !o.loop {
		t.Errorf("want the looping car hum on air, got file=%s loop=%v", o.file, o.loop)
	}
	// The selection stays the radio, so the watchdog still swaps onto it.
	if bed, _ := s.Current(); bed != Radio {
		t.Errorf("rescue changed the selection: %q", bed)
	}
	if bed, _ := s.Playing(); bed != CarHum {
		t.Errorf("on air: want carhum, got %q", bed)
	}
}

// Once the drone is standing in, further ended-media reports must not keep
// re-swapping it: the rescue is what takes the stream off the album path.
func TestAdvance_EmptyAlbumRescuesOnlyOnce(t *testing.T) {
	dir := t.TempDir()
	playing := filepath.Join(dir, "fifty-horizons", "a track.mp3")
	o := &fakeOBS{settings: map[string]any{"is_local_file": true, "local_file": playing}}
	s := NewStore(o, CarHum, dir, "twitch")
	s.Detect(context.Background())
	if err := s.Advance(context.Background()); err != nil {
		t.Fatal(err)
	}

	o.file = "" // a second swap would write the fallback path again
	if err := s.Advance(context.Background()); err != nil {
		t.Fatal(err)
	}
	if o.file != "" {
		t.Errorf("rescued a stream already on the drone: %s", o.file)
	}
}

// The bed the audio watchdog rides a radio outage out on. An outage lasts
// hours, so the album — licence-clean, and actual music — is a better degraded
// state than the drone whenever the share can supply it.
func TestSwapToFallback_PrefersTheAlbumOverTheDrone(t *testing.T) {
	dir := shareDir(t, 3)
	o := &fakeOBS{}
	s := NewStore(o, Radio, dir, "twitch")
	if err := s.SwapToFallback(context.Background()); err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(o.file) != filepath.Join(dir, "fifty-horizons") {
		t.Fatalf("fallback played %q, want a track from the album under %q", o.file, dir)
	}
	// Looping would mean OBS never reports media-ended, so the album would
	// never advance off its first track.
	if o.loop {
		t.Fatal("the fallback album must not loop")
	}
	// The swap back to the radio keys off the *selected* bed, so recording the
	// fallback as the bed would switch the watchdog off and strand the stream
	// on the album once the radio returns.
	if bed, _ := s.Current(); bed != Radio {
		t.Fatalf("selected bed after falling back: want %s, got %s", Radio, bed)
	}
}

func TestSwapToFallback_UsesTheDroneWhenTheAlbumCantPlay(t *testing.T) {
	for _, tc := range []struct{ name, dir string }{
		{"empty share", t.TempDir()},
		{"share not mounted", filepath.Join(t.TempDir(), "absent")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := &fakeOBS{}
			s := NewStore(o, Radio, tc.dir, "twitch")
			if err := s.SwapToFallback(context.Background()); err != nil {
				t.Fatal(err)
			}
			if o.file != FallbackFile {
				t.Fatalf("fallback file: want %s, got %s", FallbackFile, o.file)
			}
			// Nothing advances the drone, so it has to loop or it leaves dead air.
			if !o.loop {
				t.Fatal("the car-hum fallback must loop")
			}
		})
	}
}

// The fallback leaves the selected bed on the radio, so Advance has to know the
// album by more than the bed. Without that the outage plays one track and falls
// silent — the album=1 / tracks=0 state, which is the thing the fallback exists
// to prevent rather than to cause.
func TestSwapToFallback_AlbumKeepsAdvancing(t *testing.T) {
	const n = 3
	o := &fakeOBS{}
	s := NewStore(o, Radio, shareDir(t, n), "twitch")
	ctx := context.Background()
	if err := s.SwapToFallback(ctx); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{o.file: true}
	for range n - 1 {
		if err := s.Advance(ctx); err != nil {
			t.Fatal(err)
		}
		seen[o.file] = true
	}
	if len(seen) != n {
		t.Fatalf("fallback album played %d of %d tracks: a stalled order falls silent", len(seen), n)
	}
}

func TestSwapToRadio_EndsTheAlbumFallback(t *testing.T) {
	o := &fakeOBS{}
	s := NewStore(o, Radio, shareDir(t, 3), "twitch")
	ctx := context.Background()
	if err := s.SwapToFallback(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.SwapToRadio(ctx); err != nil {
		t.Fatal(err)
	}
	if !o.network {
		t.Fatalf("swap back left the source on the local file %q", o.file)
	}
	// A late report of the last fallback track ending must not repoint the
	// source at a track and undo the recovery.
	if err := s.Advance(ctx); err != nil {
		t.Fatal(err)
	}
	if !o.network {
		t.Fatalf("advance after the swap back put %q back on air", o.file)
	}
}

// Choosing a bed mid-outage ends the fallback: what's audible is what was
// asked for, so a track ending must not walk an album nobody selected.
func TestSet_ClearsTheAlbumFallback(t *testing.T) {
	o := &fakeOBS{}
	s := NewStore(o, Radio, shareDir(t, 3), "twitch")
	ctx := context.Background()
	if err := s.SwapToFallback(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.Set(ctx, CarHum); err != nil {
		t.Fatal(err)
	}
	if err := s.Advance(ctx); err != nil {
		t.Fatal(err)
	}
	if o.file != CarHumFile(DefaultVoicing) {
		t.Fatalf("advance walked the album after the drone was chosen: %s", o.file)
	}
}

func TestDetect_ReadsTheLiveBedFromOBS(t *testing.T) {
	dir := shareDir(t, 1)
	for _, tc := range []struct {
		name     string
		settings map[string]any
		want     Bed
	}{
		{"network stream", map[string]any{"is_local_file": false}, Radio},
		{"carhum flac", map[string]any{"is_local_file": true, "local_file": CarHumFile(DefaultVoicing)}, CarHum},
		// The watchdog's copy of the same drone means the radio is still the
		// selected bed, mid-outage. Reading it as CarHum switches the outage
		// machinery off and strands the stream on the drone after the radio
		// recovers — the restart-during-fallback case.
		{"watchdog fallback flac", map[string]any{"is_local_file": true, "local_file": FallbackFile}, Radio},
		// OBS boots on the album fallback, which leaves the radio selected for
		// the watchdog to bring on air.
		{"album track", map[string]any{"is_local_file": true, "local_file": filepath.Join(dir, "fifty-horizons", "a track.mp3")}, Radio},
		// A sibling directory whose name merely prefixes the share must not
		// read as the album.
		{"lookalike dir", map[string]any{"is_local_file": true, "local_file": dir + "-old/x.mp3"}, CarHum},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := NewStore(&fakeOBS{settings: tc.settings}, Radio, dir, "twitch")
			s.Detect(context.Background())
			if bed, _ := s.Current(); bed != tc.want {
				t.Fatalf("detected %s, want %s", bed, tc.want)
			}
		})
	}
}

// OBS boots every platform on an album track, so a fresh boot reads as the
// radio selected with the album fallback on air — and the play order has to be
// built here, from FallbackAlbums only, or the boot track ends in silence.
func TestDetect_BootOnTheAlbumIsTheRadioOnItsFallback(t *testing.T) {
	dir := shareOf(t, map[string]int{"fifty-horizons": 3, "personal-mix": 3})
	playing := filepath.Join(dir, "fifty-horizons", "a track.mp3")
	o := &fakeOBS{settings: map[string]any{"is_local_file": true, "local_file": playing}}
	s := NewStore(o, CarHum, dir, "twitch")
	s.Detect(context.Background())

	if bed, _ := s.Current(); bed != Radio {
		t.Fatalf("selected bed = %s; want radio", bed)
	}
	if bed, track := s.Playing(); bed != Album || track != playing {
		t.Fatalf("Playing() = %s, %q; want the album fallback on %q", bed, track, playing)
	}
	for i := range 5 {
		if err := s.Advance(context.Background()); err != nil {
			t.Fatal(err)
		}
		if filepath.Base(filepath.Dir(o.file)) != "fifty-horizons" || (i == 0 && o.file == playing) {
			t.Fatalf("advance %d wrote %q; want another fifty-horizons track", i, o.file)
		}
	}
}

// An album fallback detected with no tracks to scan is reported honestly rather
// than crashing or claiming a track — the console shows what's on air.
func TestDetect_AlbumWithNoTracksStillReportsTheBed(t *testing.T) {
	dir := t.TempDir()
	o := &fakeOBS{settings: map[string]any{
		"is_local_file": true,
		"local_file":    filepath.Join(dir, "gone", "x.mp3"),
	}}
	s := NewStore(o, CarHum, dir, "twitch")
	s.Detect(context.Background())
	if bed, track := s.Playing(); bed != Album || track != "" {
		t.Fatalf("Playing() = %s, %q; want the album fallback with no track", bed, track)
	}
}

// A tripbot restart must not report the default channel while OBS plays another
// one — the scene's `input` URL is the only record of which is tuned.
func TestDetect_ReadsTheStationFromTheSourceURL(t *testing.T) {
	o := &fakeOBS{settings: map[string]any{
		"is_local_file": false,
		"input":         "https://radio.example/jazz.mp3",
	}}
	s := NewStore(o, CarHum, t.TempDir(), "twitch")
	s.Detect(context.Background())
	if s.Station() != "jazz" {
		t.Fatalf("station = %q, want jazz", s.Station())
	}
}

func TestDetect_KeepsTheSeedWhenOBSIsUnreachable(t *testing.T) {
	s := NewStore(&fakeOBS{err: errors.New("nope")}, Radio, t.TempDir(), "twitch")
	s.Detect(context.Background())
	if bed, _ := s.Current(); bed != Radio {
		t.Fatalf("want the seed bed retained, got %s", bed)
	}
}

// Wrapping the play order reshuffles it, and a plain shuffle can deal the track
// that just finished straight back to the front — an immediate repeat is the
// one ordering a listener notices. Walk several full wraps so the reshuffle is
// exercised many times rather than once.
func TestAdvance_NeverRepeatsATrackAcrossTheWrap(t *testing.T) {
	o := &fakeOBS{}
	s := NewStore(o, Radio, shareDir(t, 3), "twitch")
	if err := s.SwapToFallback(context.Background()); err != nil {
		t.Fatal(err)
	}
	prev := o.file
	for i := 0; i < 60; i++ {
		if err := s.Advance(context.Background()); err != nil {
			t.Fatal(err)
		}
		if o.file == prev {
			t.Fatalf("advance %d replayed %q back-to-back", i, filepath.Base(o.file))
		}
		prev = o.file
	}
}

// The share holds two filename conventions — Bandcamp's tagged
// "<artist> - <album> - <NN Title>" and a bare numbered title — and chat asks
// what a song is, not where it sits on a disc.
func TestParseTrack(t *testing.T) {
	for _, tc := range []struct {
		path string
		want Track
	}{{
		path: "/music/streambeats-synthwave-breaker/StreamBeats by Harris Heller - Breaker - 21 Holosmith.flac",
		want: Track{Title: "Holosmith", Album: "Breaker", Artist: "StreamBeats by Harris Heller"},
	}, {
		// A hyphen inside the title survives: only the first two fields are the
		// artist and the album, whatever follows is one title.
		path: "/music/x/Artist - Album - 04 Cross - Country.mp3",
		want: Track{Title: "Cross - Country", Album: "Album", Artist: "Artist"},
	}, {
		path: "/music/fifty-horizons/001 Maine - Atlantic Dawn.mp3",
		want: Track{Title: "Maine - Atlantic Dawn"},
	}, {
		path: "/music/x/07 Petals.mp3",
		want: Track{Title: "Petals"},
	}, {
		path: "/music/x/Untitled.mp3",
		want: Track{Title: "Untitled"},
	}, {
		// All-number filename: dropping the number would leave nothing to say.
		path: "/music/x/07.mp3",
		want: Track{Title: "07"},
	}, {
		path: "",
		want: Track{},
	}} {
		if got := ParseTrack(tc.path); got != tc.want {
			t.Errorf("ParseTrack(%q) = %+v, want %+v", tc.path, got, tc.want)
		}
	}
}

// waitFor polls until cond holds, so a test can wait on a scheduled switch
// landing without sleeping for the whole delay.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

// The reason the delay exists: a mis-click corrected inside the window lands
// once, on the correction. Queueing both would play the mistake for a moment,
// which on a live stream is the disruption being avoided.
func TestSchedule_SupersedesASwitchStillWaiting(t *testing.T) {
	switchDelay = 50 * time.Millisecond
	t.Cleanup(func() { switchDelay = 0 })

	o := &fakeOBS{}
	s := NewStore(o, Radio, shareDir(t, 3), "twitch")
	ctx := context.Background()
	if err := s.Set(ctx, Radio); err != nil {
		t.Fatal(err)
	}
	if err := s.Set(ctx, CarHum); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the switch to land", func() bool {
		bed, _ := s.Current()
		return bed == CarHum
	})

	// Reading the fake after observing the store settle is ordered by the store's
	// own lock, so there's no race with the timer goroutine.
	if o.network {
		t.Errorf("the superseded radio switch reached OBS: url=%q", o.url)
	}
	if o.file != CarHumFile(DefaultVoicing) {
		t.Errorf("OBS is playing %q, want the car hum", o.file)
	}
}

// While a tune waits, the store still reports the channel that's audible. The
// pending one is a separate answer, so nothing can report a station nobody is
// hearing (the same rule the rollback has always kept).
func TestSchedule_ReportsThePendingTuneWithoutClaimingIt(t *testing.T) {
	switchDelay = 50 * time.Millisecond
	t.Cleanup(func() { switchDelay = 0 })

	o := &fakeOBS{}
	s := NewStore(o, CarHum, shareDir(t, 3), "twitch")
	if err := s.SetStation(context.Background(), "lofi"); err != nil {
		t.Fatal(err)
	}

	sw, ok := s.Pending()
	if !ok {
		t.Fatal("want the tune reported as pending")
	}
	if sw.Bed != Radio || sw.Station != "lofi" {
		t.Errorf("pending = %+v, want the radio bed on lofi", sw)
	}
	if sw.At.Before(time.Now()) {
		t.Errorf("pending lands at %v, already in the past", sw.At)
	}
	if got := s.Station(); got == "lofi" {
		t.Error("the pending station is being reported as the live one")
	}

	// Wait on the pending flag, not the bed. applyPending records the station,
	// switches the bed, and only then clears pending — so the cleared flag is the
	// one signal that the whole tune has landed. Waiting on the bed instead
	// asserts inside the window between the bed moving and the flag clearing,
	// which fails a store that is working correctly.
	waitFor(t, "the tune to land", func() bool {
		_, pending := s.Pending()
		return !pending
	})
	if got := s.Station(); got != "lofi" {
		t.Errorf("station = %q after the tune landed, want lofi", got)
	}
	if bed, _ := s.Current(); bed != Radio {
		t.Errorf("bed = %q after the tune landed, want radio", bed)
	}
}

// Everything that names what a viewer is hearing — !song, !audio, the console's
// now-playing line — reads Playing, and during an outage that is not the
// selection: the watchdog puts the album on air and leaves the radio selected, so
// a reader of the selection announces a station, and a radio track, nobody is
// hearing.
func TestPlaying_NamesTheFallbackWhileTheSelectionStandsRadio(t *testing.T) {
	dir := shareDir(t, 3)
	o := &fakeOBS{}
	s := NewStore(o, Radio, dir, "twitch")
	ctx := context.Background()
	if bed, _ := s.Playing(); bed != Radio {
		t.Fatalf("playing bed before the outage: want %s, got %s", Radio, bed)
	}
	if err := s.SwapToFallback(ctx); err != nil {
		t.Fatal(err)
	}
	bed, track := s.Playing()
	if bed != Album {
		t.Fatalf("playing bed on the fallback: want %s, got %s", Album, bed)
	}
	if track != o.file {
		t.Fatalf("playing track %q, want the one on air %q", track, o.file)
	}
	if s.PlayingAlbum() != "fifty-horizons" {
		t.Fatalf("playing album on the fallback: got %q", s.PlayingAlbum())
	}
	if sel, _ := s.Current(); sel != Radio {
		t.Fatalf("selected bed: want %s, got %s", Radio, sel)
	}
	if err := s.SwapToRadio(ctx); err != nil {
		t.Fatal(err)
	}
	if bed, track := s.Playing(); bed != Radio || track != "" {
		t.Fatalf("playing after recovery: want %s with no track, got %s %q", Radio, bed, track)
	}
}

// The drone is the fallback whenever the share can't supply a track, and it
// misnames the audio in exactly the same way.
func TestPlaying_NamesTheDroneFallback(t *testing.T) {
	o := &fakeOBS{}
	s := NewStore(o, Radio, t.TempDir(), "twitch")
	if err := s.SwapToFallback(context.Background()); err != nil {
		t.Fatal(err)
	}
	if bed, track := s.Playing(); bed != CarHum || track != "" {
		t.Fatalf("playing on the drone fallback: want %s with no track, got %s %q", CarHum, bed, track)
	}
}

// albumsOf is the store's album library, sorted.
func albumsOf(s *Store) []string { return slices.Sorted(maps.Keys(s.albumTracks())) }

// writeIndex emits the album track index bin/stage-streambeats produces, at a
// path of its own so the test can point a store at it with no share mounted —
// which is the shape a tripbot pod runs in.
func writeIndex(t *testing.T, byAlbum map[string][]string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "index.json")
	raw, err := json.Marshal(byAlbum)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// The index is the whole music dependency: with no share on disk at all, the
// fallback still builds and plays, because every path tripbot handles is a
// string it passes to OBS. Only FallbackAlbums reach the play order — an album
// that lands on the share by mistake never does.
func TestFallbackRunsOffTheIndexWithNoShareMounted(t *testing.T) {
	index := writeIndex(t, map[string][]string{
		"streambeats-lofi-gold":     {MusicDir + "/streambeats-lofi-gold/b two.mp3", MusicDir + "/streambeats-lofi-gold/a one.mp3"},
		"streambeats-lofi-secluded": {MusicDir + "/streambeats-lofi-secluded/a one.mp3"},
		"streambeats-lofi-empty":    {},
		"personal-mix":              {MusicDir + "/personal-mix/a one.mp3"},
	})
	o := &fakeOBS{}
	s := NewStore(o, Radio, "", "twitch").WithIndex(index) // "" is MusicDir, which the test machine has no more than a pod does

	if got, want := albumsOf(s), []string{"personal-mix", "streambeats-lofi-gold", "streambeats-lofi-secluded"}; !slices.Equal(got, want) {
		t.Fatalf("library = %v, want %v (an empty album isn't one)", got, want)
	}
	ctx := context.Background()
	if err := s.SwapToFallback(ctx); err != nil {
		t.Fatal(err)
	}
	played := map[string]bool{o.file: true}
	for range 6 {
		if err := s.Advance(ctx); err != nil {
			t.Fatal(err)
		}
		played[o.file] = true
	}
	want := map[string]bool{
		MusicDir + "/streambeats-lofi-gold/a one.mp3":     true,
		MusicDir + "/streambeats-lofi-gold/b two.mp3":     true,
		MusicDir + "/streambeats-lofi-secluded/a one.mp3": true,
	}
	if !maps.Equal(played, want) {
		t.Fatalf("fallback played %v, want exactly the StreamBeats tracks %v", slices.Sorted(maps.Keys(played)), slices.Sorted(maps.Keys(want)))
	}
}

// A missing index leaves the album bed empty rather than erroring the process,
// which is the state the optional ConfigMap mount produces before any music is
// staged. The share is the fallback, so a laptop run with the real directory
// mounted is unaffected.
func TestMissingIndexFallsBackToTheShare(t *testing.T) {
	dir := shareDir(t, 2)
	s := NewStore(&fakeOBS{}, CarHum, dir, "twitch").WithIndex(filepath.Join(t.TempDir(), "absent.json"))
	if got, want := albumsOf(s), []string{"fifty-horizons"}; !slices.Equal(got, want) {
		t.Fatalf("library = %v, want %v", got, want)
	}

	s = NewStore(&fakeOBS{}, CarHum, "/nonexistent-share", "twitch").WithIndex(filepath.Join(t.TempDir(), "absent.json"))
	if got := albumsOf(s); len(got) != 0 {
		t.Fatalf("library = %v, want none", got)
	}
	o := &fakeOBS{}
	s.obs = o
	if err := s.SwapToFallback(context.Background()); err != nil || o.file != FallbackFile {
		t.Fatalf("fallback with no library played %q (err %v); want the drone", o.file, err)
	}
}

// Every swap onto the fallback and every boot reads the library, so the index is read once and served
// from the cache until its mtime moves. Rewriting the file under a pinned mtime
// and size proves the repeat read never opened it; moving the mtime proves an
// album staged while the bot runs still shows up.
func TestAlbumIndexIsCachedUntilItsMtimeMoves(t *testing.T) {
	index := writeIndex(t, map[string][]string{"aaa": {MusicDir + "/aaa/one.mp3"}})
	fi, err := os.Stat(index)
	if err != nil {
		t.Fatal(err)
	}
	s := NewStore(&fakeOBS{}, CarHum, "/nonexistent-share", "twitch").WithIndex(index)

	if got, want := albumsOf(s), []string{"aaa"}; !slices.Equal(got, want) {
		t.Fatalf("library = %v, want %v", got, want)
	}

	// Same byte count, so only a read of the file could tell the two apart.
	rewrite := func(byAlbum map[string][]string, mod time.Time) {
		t.Helper()
		raw, err := json.Marshal(byAlbum)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(index, raw, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(index, mod, mod); err != nil {
			t.Fatal(err)
		}
	}

	rewrite(map[string][]string{"bbb": {MusicDir + "/bbb/one.mp3"}}, fi.ModTime())
	if got, want := albumsOf(s), []string{"aaa"}; !slices.Equal(got, want) {
		t.Fatalf("library with the index rewritten under its own mtime = %v, want the cached %v", got, want)
	}

	rewrite(map[string][]string{"bbb": {MusicDir + "/bbb/one.mp3"}}, fi.ModTime().Add(time.Hour))
	if got, want := albumsOf(s), []string{"bbb"}; !slices.Equal(got, want) {
		t.Fatalf("library after the index mtime moved = %v, want %v", got, want)
	}
}

func TestSetVoicing_PlaysThatRenderAndSelectsTheDrone(t *testing.T) {
	o := &fakeOBS{}
	s := NewStore(o, Radio, shareDir(t, 2), "twitch")
	if err := s.SetVoicing(context.Background(), "highway"); err != nil {
		t.Fatal(err)
	}
	// Picking a voicing you can't hear isn't a thing anyone means, so it
	// selects the drone too — the same shape as SetStation.
	if bed, _ := s.Current(); bed != CarHum {
		t.Fatalf("bed: want %s, got %s", CarHum, bed)
	}
	if want := CarHumFile("highway"); o.file != want {
		t.Fatalf("file: want %s, got %s", want, o.file)
	}
	if s.Voicing() != "highway" {
		t.Fatalf("voicing: want highway, got %s", s.Voicing())
	}
}

func TestSetVoicing_RefusesAnUnrenderedName(t *testing.T) {
	o := &fakeOBS{}
	s := NewStore(o, CarHum, shareDir(t, 2), "twitch")
	// The image ships one FLAC per voicing; a name it never rendered is a path
	// OBS cannot open, which is silence.
	if err := s.SetVoicing(context.Background(), "spaceship"); err == nil {
		t.Fatal("want an error for a voicing the image does not ship")
	}
	if s.Voicing() != DefaultVoicing {
		t.Fatalf("a refused voicing changed the selection: %s", s.Voicing())
	}
}

func TestVoicing_SurvivesASwitchAwayAndBack(t *testing.T) {
	o := &fakeOBS{}
	s := NewStore(o, CarHum, shareDir(t, 2), "twitch")
	ctx := context.Background()
	if err := s.SetVoicing(ctx, "mountain"); err != nil {
		t.Fatal(err)
	}
	if err := s.Set(ctx, Radio); err != nil {
		t.Fatal(err)
	}
	if err := s.Set(ctx, CarHum); err != nil {
		t.Fatal(err)
	}
	if want := CarHumFile("mountain"); o.file != want {
		t.Fatalf("the drone returned to the default instead of the chosen voicing: %s", o.file)
	}
}

func TestDetect_ReadsTheVoicingOffTheDroneFile(t *testing.T) {
	for _, tc := range []struct {
		name string
		file string
		want string
	}{
		{"a shipped render", CarHumFile("backroad"), "backroad"},
		// The watchdog's copy is the same audio under its own name. It means an
		// outage, not a choice, so it must leave the selection alone.
		{"the watchdog's fallback", FallbackFile, DefaultVoicing},
		{"a name the image never rendered", filepath.Join(CarHumDir, "car-hum-spaceship.flac"), DefaultVoicing},
		{"a file outside the drone directory", "/opt/tripbot/assets/other/car-hum-highway.flac", DefaultVoicing},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := &fakeOBS{settings: map[string]any{"is_local_file": true, "local_file": tc.file}}
			s := NewStore(o, Radio, shareDir(t, 1), "twitch")
			s.Detect(context.Background())
			if s.Voicing() != tc.want {
				t.Fatalf("voicing: want %s, got %s", tc.want, s.Voicing())
			}
		})
	}
}

// scanTracks picks up every track extension the localize index can name,
// Ogg Opus included, and nothing else in an album directory.
func TestScanTracksExtensions(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.mp3", "b.flac", "c.m4a", "d.ogg", "e.OPUS", "cover.jpg", "f.mp3@SynoEAStream"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := scanTracks(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, p := range got {
		names = append(names, filepath.Base(p))
	}
	if want := []string{"a.mp3", "b.flac", "c.m4a", "d.ogg", "e.OPUS"}; !slices.Equal(names, want) {
		t.Fatalf("scanTracks = %v, want %v", names, want)
	}
}
