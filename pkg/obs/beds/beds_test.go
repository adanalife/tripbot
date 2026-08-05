package beds

import (
	"context"
	"errors"
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
func TestMain(m *testing.M) {
	advanceDebounce = 0
	switchDelay = 0
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

func TestAlbums_ListsTrackBearingSubdirsOnly(t *testing.T) {
	s := NewStore(&fakeOBS{}, CarHum, realShare(t), "twitch")
	// Sorted, so the picker and chat's option list read the same order every time.
	want := []string{
		"fifty-horizons",
		"streambeats-ambient-diamonds",
		"streambeats-lofi-gold",
		"streambeats-lofi-secluded",
		"streambeats-synthwave-breaker",
		"streambeats-synthwave-rose",
	}
	if got := s.Albums(); !slices.Equal(got, want) {
		t.Errorf("albums: want %v, got %v", want, got)
	}
}

// The naming convention IS the grouping: a prefix covering more than one album is
// a group, so albums join one by being named like their siblings.
func TestGroups_ArePrefixesCoveringMoreThanOneAlbum(t *testing.T) {
	s := NewStore(&fakeOBS{}, CarHum, realShare(t), "twitch")
	// "streambeats" (5), "streambeats-lofi" (2), "streambeats-synthwave" (2).
	// "streambeats-ambient" covers only Diamonds, so it isn't offered as a group —
	// though it still resolves, since a prefix over one album is unambiguous.
	// "fifty" covers one album and is likewise not a group.
	want := []string{"streambeats", "streambeats-lofi", "streambeats-synthwave"}
	if got := s.Groups(); !slices.Equal(got, want) {
		t.Errorf("groups: want %v, got %v", want, got)
	}
}

func TestResolveAlbum(t *testing.T) {
	s := NewStore(&fakeOBS{}, CarHum, realShare(t), "twitch")
	for _, tc := range []struct{ arg, want string }{
		{"streambeats-lofi-gold", "streambeats-lofi-gold"}, // exact album
		{"gold", "streambeats-lofi-gold"},                  // unique trailing segment
		{"rose", "streambeats-synthwave-rose"},             // the shorthand chat will use
		{"streambeats", "streambeats"},                     // the all-of-it group
		{"streambeats-lofi", "streambeats-lofi"},           // a narrower group
		{"streambeats-ambient", "streambeats-ambient"},     // a prefix over one album
		{"fifty-horizons", "fifty-horizons"},               // exact, with no prefix
		{"horizons", "fifty-horizons"},                     // reachable by suffix
		{"streambeats-lo", ""},                             // a partial segment is a typo
		{"lofi", ""},                                       // an interior segment is not a prefix
		{"empty", ""},                                      // trackless dir isn't an album
		{"groovesalad", ""},                                // a station is not an album
		{"", ""},                                           // no argument resolves to nothing
	} {
		if got := s.ResolveAlbum(tc.arg); got != tc.want {
			t.Errorf("ResolveAlbum(%q): want %q, got %q", tc.arg, tc.want, got)
		}
	}
}

func TestResolveAlbum_AmbiguousShorthandResolvesToNothing(t *testing.T) {
	// There are two different StreamBeats albums called "Midnight", one synthwave
	// and one hip-hop, so this is the live case rather than a hypothetical. Once
	// both are on the share "midnight" has stopped naming one of them, and
	// guessing would switch the stream to whichever sorted first.
	s := NewStore(&fakeOBS{}, CarHum, shareOf(t, map[string]int{
		"streambeats-synthwave-midnight": 2, "streambeats-hiphop-midnight": 2,
	}), "twitch")
	if got := s.ResolveAlbum("midnight"); got != "" {
		t.Errorf("ambiguous shorthand: want %q, got %q", "", got)
	}
	if got := s.ResolveAlbum("streambeats-hiphop-midnight"); got != "streambeats-hiphop-midnight" {
		t.Errorf("exact name must still win over the ambiguity: got %q", got)
	}
}

// A group plays every album under it. Unshuffled, they arrive album by album,
// which is what makes "I've heard enough of this one" a judgement you can make.
func TestSetAlbum_GroupPlaysEveryAlbumUnderItInOrder(t *testing.T) {
	dir := realShare(t)
	s := NewStore(&fakeOBS{}, CarHum, dir, "twitch")
	if err := s.SetShuffle(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if err := s.SetAlbum(context.Background(), "streambeats-lofi"); err != nil {
		t.Fatal(err)
	}
	// gold (2) then secluded (4), in sorted directory order — not interleaved.
	var seen []string
	for i := 0; i < 6; i++ {
		_, track := s.Current()
		seen = append(seen, filepath.Base(filepath.Dir(track)))
		if err := s.Advance(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	want := []string{
		"streambeats-lofi-gold", "streambeats-lofi-gold",
		"streambeats-lofi-secluded", "streambeats-lofi-secluded",
		"streambeats-lofi-secluded", "streambeats-lofi-secluded",
	}
	if !slices.Equal(seen, want) {
		t.Errorf("group play order:\n want %v\n  got %v", want, seen)
	}
}

// The whole point of the group for Dana: knowing which album a track came from,
// so an album can be dropped from the rotation. The selection can't answer that.
func TestPlayingAlbum_NamesTheTracksAlbumNotTheSelection(t *testing.T) {
	dir := realShare(t)
	s := NewStore(&fakeOBS{}, CarHum, dir, "twitch")
	if err := s.SetShuffle(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if err := s.SetAlbum(context.Background(), "streambeats"); err != nil {
		t.Fatal(err)
	}
	if got := s.Album(); got != "streambeats" {
		t.Fatalf("selection: want streambeats, got %q", got)
	}
	if got := s.PlayingAlbum(); got != "streambeats-ambient-diamonds" {
		t.Errorf("playing album: want streambeats-ambient-diamonds, got %q", got)
	}
	// Walk into the next album and the answer follows the audio.
	for range 2 {
		if err := s.Advance(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if got := s.PlayingAlbum(); got != "streambeats-lofi-gold" {
		t.Errorf("playing album after advancing: want streambeats-lofi-gold, got %q", got)
	}
}

func TestPlayingAlbum_EmptyOffTheAlbumBed(t *testing.T) {
	s := NewStore(&fakeOBS{}, CarHum, realShare(t), "twitch")
	if got := s.PlayingAlbum(); got != "" {
		t.Errorf("car hum has no album, got %q", got)
	}
}

// Sequential order must survive a wrap — the re-shuffle on wrap is what keeps a
// shuffled order fresh, and running it on a sequential one would silently turn
// shuffle back on after one pass.
func TestAdvance_SequentialOrderRepeatsRatherThanReshuffling(t *testing.T) {
	dir := shareOf(t, map[string]int{"streambeats-lofi-gold": 4})
	s := NewStore(&fakeOBS{}, CarHum, dir, "twitch")
	if err := s.SetShuffle(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if err := s.SetAlbum(context.Background(), "streambeats-lofi-gold"); err != nil {
		t.Fatal(err)
	}
	var first, second []string
	for i := 0; i < 8; i++ {
		_, track := s.Current()
		if i < 4 {
			first = append(first, filepath.Base(track))
		} else {
			second = append(second, filepath.Base(track))
		}
		if err := s.Advance(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if !slices.Equal(first, second) {
		t.Errorf("second pass differs from the first:\n %v\n %v", first, second)
	}
	if !slices.IsSorted(first) {
		t.Errorf("sequential order should be sorted, got %v", first)
	}
}

// Toggling shuffle mid-track must not jump the audio: the track on air stays at
// the front of the rebuilt order.
func TestSetShuffle_KeepsTheTrackOnAir(t *testing.T) {
	dir := shareOf(t, map[string]int{"streambeats-lofi-gold": 8})
	s := NewStore(&fakeOBS{}, CarHum, dir, "twitch")
	if err := s.SetAlbum(context.Background(), "streambeats-lofi-gold"); err != nil {
		t.Fatal(err)
	}
	_, before := s.Current()
	if err := s.SetShuffle(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if _, after := s.Current(); after != before {
		t.Errorf("track changed under the toggle: %q -> %q", before, after)
	}
	if s.Shuffle() {
		t.Error("shuffle should be off")
	}
	if err := s.SetShuffle(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if _, after := s.Current(); after != before {
		t.Errorf("track changed turning shuffle back on: %q -> %q", before, after)
	}
}

func TestSetAlbum_PlaysOnlyThatAlbumsTracks(t *testing.T) {
	o := &fakeOBS{}
	dir := realShare(t)
	s := NewStore(o, CarHum, dir, "twitch")

	if err := s.SetAlbum(context.Background(), "streambeats-ambient-diamonds"); err != nil {
		t.Fatal(err)
	}
	if got := s.Album(); got != "streambeats-ambient-diamonds" {
		t.Errorf("selected album: want ambient-diamonds, got %q", got)
	}
	bed, _ := s.Current()
	if bed != Album {
		t.Errorf("picking an album must select the album bed, got %q", bed)
	}
	// Walk the whole order; every track must come from the selected album. The
	// bug this catches is the play order still spanning the share, which sounds
	// fine for one track and then wanders into another album.
	want := filepath.Join(dir, "streambeats-ambient-diamonds")
	for i := 0; i < 6; i++ {
		if _, track := s.Current(); filepath.Dir(track) != want {
			t.Fatalf("track %d came from outside the album: %s", i, track)
		}
		if err := s.Advance(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSetAlbum_UnknownAlbumIsRefusedAndChangesNothing(t *testing.T) {
	o := &fakeOBS{}
	s := NewStore(o, CarHum, realShare(t), "twitch")
	if err := s.SetAlbum(context.Background(), "streambeats-edm-nocturnal"); err == nil {
		t.Fatal("expected an error for an album that isn't on the share")
	}
	if got := s.Album(); got != "" {
		t.Errorf("a refused album must not be recorded, got %q", got)
	}
	if bed, _ := s.Current(); bed != CarHum {
		t.Errorf("a refused album must not switch the bed, got %q", bed)
	}
}

func TestSetAlbum_EmptyWidensToTheWholeShare(t *testing.T) {
	dir := realShare(t)
	s := NewStore(&fakeOBS{}, CarHum, dir, "twitch")
	if err := s.SetAlbum(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	// 3+2+4+2 tracks, and neither loose root file.
	seen := map[string]bool{}
	for i := 0; i < 11; i++ {
		_, track := s.Current()
		seen[track] = true
		if filepath.Dir(track) == filepath.Clean(dir) {
			t.Fatalf("loose share-root file played as a track: %s", track)
		}
		if err := s.Advance(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if len(seen) != 11 {
		t.Errorf("whole-share order: want 11 distinct tracks, got %d", len(seen))
	}
}

func TestDetect_RecoversTheAlbumFromThePlayingTrack(t *testing.T) {
	// A restart mid-album must come back scoped to that album. Reading only the
	// bed back would widen the order to the whole share, so the next advance
	// wanders out of what's on air.
	dir := realShare(t)
	playing := filepath.Join(dir, "streambeats-lofi-secluded", "b track.mp3")
	o := &fakeOBS{settings: map[string]any{"is_local_file": true, "local_file": playing}}
	s := NewStore(o, CarHum, dir, "twitch")
	s.Detect(context.Background())

	if got := s.Album(); got != "streambeats-lofi-secluded" {
		t.Fatalf("recovered album: want lofi-secluded, got %q", got)
	}
	want := filepath.Join(dir, "streambeats-lofi-secluded")
	if _, track := s.Current(); filepath.Dir(track) != want {
		t.Errorf("play order not scoped to the recovered album: %s", track)
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

func TestSet_SomaFMUsesNetworkMode(t *testing.T) {
	o := &fakeOBS{}
	s := NewStore(o, CarHum, shareDir(t, 2), "twitch")
	if err := s.Set(context.Background(), SomaFM); err != nil {
		t.Fatal(err)
	}
	if !o.network {
		t.Fatal("somafm should put the source in network mode")
	}
	if bed, track := s.Current(); bed != SomaFM || track != "" {
		t.Fatalf("current: want somafm/no track, got %s/%s", bed, track)
	}
}

// The station is what makes the SomaFM bed a choice rather than one stream, so
// tuning has to reach OBS as a URL and select the bed in one move — nobody means
// "tune a station I'm not playing".
func TestSetStation_TunesAndSelectsTheSomaFMBed(t *testing.T) {
	o := &fakeOBS{}
	s := NewStore(o, CarHum, shareDir(t, 2), "twitch")
	if err := s.SetStation(context.Background(), "dronezone"); err != nil {
		t.Fatal(err)
	}
	if o.url != StreamURL("dronezone") {
		t.Fatalf("obs url = %q, want %q", o.url, StreamURL("dronezone"))
	}
	if bed, _ := s.Current(); bed != SomaFM {
		t.Fatalf("bed = %s, want somafm", bed)
	}
	if s.Station() != "dronezone" {
		t.Fatalf("station = %q, want dronezone", s.Station())
	}
}

// A station OBS wouldn't accept must not become the one we report: !song and the
// console's now-playing line would then name a channel nobody is hearing.
func TestSetStation_RollsBackWhenOBSRejectsIt(t *testing.T) {
	o := &fakeOBS{err: errors.New("obs unreachable")}
	s := NewStore(o, SomaFM, t.TempDir(), "twitch")
	if err := s.SetStation(context.Background(), "dronezone"); err == nil {
		t.Fatal("want an error when OBS rejects the tune")
	}
	if s.Station() != DefaultStation {
		t.Fatalf("station = %q, want the previous %q", s.Station(), DefaultStation)
	}
}

func TestSetStation_RejectsAnUnknownChannel(t *testing.T) {
	s := NewStore(&fakeOBS{}, SomaFM, t.TempDir(), "twitch")
	if err := s.SetStation(context.Background(), "not-a-channel"); err == nil {
		t.Fatal("want an error for a channel SomaFM doesn't have")
	}
}

// Switching away and back has to land on the tuned station, not the default:
// the station outlives the bed.
func TestSet_SomaFMReturnsToTheTunedStation(t *testing.T) {
	o := &fakeOBS{}
	s := NewStore(o, SomaFM, shareDir(t, 2), "twitch")
	if err := s.SetStation(context.Background(), "u80s"); err != nil {
		t.Fatal(err)
	}
	if err := s.Set(context.Background(), CarHum); err != nil {
		t.Fatal(err)
	}
	if err := s.Set(context.Background(), SomaFM); err != nil {
		t.Fatal(err)
	}
	if o.url != StreamURL("u80s") {
		t.Fatalf("obs url = %q, want the tuned %q", o.url, StreamURL("u80s"))
	}
}

func TestStationFromURL(t *testing.T) {
	for _, tc := range []struct{ url, want string }{
		{StreamURL("dronezone"), "dronezone"},
		// The obs repo's scene config pins an edge host and its own bitrate; both
		// have to read back as the station.
		{"https://ice4.somafm.com/gsclassic-128-mp3", "gsclassic"},
		{"https://ice2.somafm.com/dronezone-256-mp3", "dronezone"},
		// Ids with digits are still whole ids.
		{StreamURL("7soul"), "7soul"},
		{StreamURL("u80s"), "u80s"},
		{"https://ice.somafm.com/nosuchchannel-128-mp3", ""},
		// "live" is a real channel id, so a non-SomaFM URL that happens to end in
		// one must not read as a station.
		{"rtmp://example.com/live", ""},
		{"", ""},
	} {
		if got := stationFromURL(tc.url); got != tc.want {
			t.Errorf("stationFromURL(%q) = %q, want %q", tc.url, got, tc.want)
		}
	}
}

func TestSet_CarHumLoops(t *testing.T) {
	o := &fakeOBS{}
	s := NewStore(o, SomaFM, shareDir(t, 2), "twitch")
	if err := s.Set(context.Background(), CarHum); err != nil {
		t.Fatal(err)
	}
	if o.file != CarHumFile {
		t.Fatalf("carhum file: want %s, got %s", CarHumFile, o.file)
	}
	// A drone that stops leaves dead air — it has to loop.
	if !o.loop {
		t.Fatal("carhum must loop")
	}
}

func TestSet_AlbumPlaysATrackUnlooped(t *testing.T) {
	dir := shareDir(t, 3)
	o := &fakeOBS{}
	s := NewStore(o, CarHum, dir, "twitch")
	if err := s.Set(context.Background(), Album); err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(o.file) != filepath.Join(dir, "fifty-horizons") {
		t.Fatalf("album track %q not from the album directory under %q", o.file, dir)
	}
	// Looping an album track means OBS never reports media-ended, so the
	// advance never fires and the stream sticks on one song forever.
	if o.loop {
		t.Fatal("album tracks must not loop")
	}
	if _, track := s.Current(); track != o.file {
		t.Fatalf("current track: want %s, got %s", o.file, track)
	}
}

func TestSet_AlbumSkipsNonAudioFiles(t *testing.T) {
	dir := shareDir(t, 1)
	o := &fakeOBS{}
	s := NewStore(o, CarHum, dir, "twitch")
	if err := s.Set(context.Background(), Album); err != nil {
		t.Fatal(err)
	}
	if filepath.Ext(o.file) != ".mp3" {
		t.Fatalf("picked a non-audio file: %s", o.file)
	}
}

func TestSet_AlbumIgnoresLooseFilesAtTheShareRoot(t *testing.T) {
	// The real share keeps carsounds.m4a — 556MB, nine hours — beside the album
	// directories. Treating it as a track would put it on the stream and stall
	// the rotation until it ended.
	dir := shareDir(t, 3)
	o := &fakeOBS{}
	s := NewStore(o, CarHum, dir, "twitch")
	for i := 0; i < 6; i++ {
		if err := s.Set(context.Background(), Album); err != nil {
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

func TestSet_AlbumWithOnlyLooseRootFilesFails(t *testing.T) {
	// A share with audio but no album directory has nothing playable — better a
	// refused switch than a 556MB archive on the stream.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "carsounds.m4a"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	s := NewStore(&fakeOBS{}, CarHum, dir, "twitch")
	if err := s.Set(context.Background(), Album); err == nil {
		t.Fatal("a share with no album directory should refuse the album bed")
	}
}

func TestSet_AlbumWithNoTracksFails(t *testing.T) {
	o := &fakeOBS{}
	s := NewStore(o, CarHum, t.TempDir(), "twitch")
	if err := s.Set(context.Background(), Album); err == nil {
		t.Fatal("empty share should fail rather than silence the stream")
	}
	// The previous bed must keep playing and keep being reported.
	if bed, _ := s.Current(); bed != CarHum {
		t.Fatalf("bed after a failed switch: want carhum, got %s", bed)
	}
}

func TestSet_FailedSwitchKeepsReportingTheOldBed(t *testing.T) {
	o := &fakeOBS{err: errors.New("obs unreachable")}
	s := NewStore(o, SomaFM, shareDir(t, 2), "twitch")
	if err := s.Set(context.Background(), CarHum); err == nil {
		t.Fatal("want an error when OBS rejects the switch")
	}
	if bed, _ := s.Current(); bed != SomaFM {
		t.Fatalf("bed after a rejected switch: want somafm, got %s", bed)
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
	s := NewStore(o, CarHum, shareDir(t, n), "twitch")
	if err := s.Set(context.Background(), Album); err != nil {
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
	s := NewStore(o, CarHum, shareOf(t, map[string]int{"streambeats-lofi-gold": 4}), "twitch")
	ctx := context.Background()
	if err := s.SetShuffle(ctx, false); err != nil {
		t.Fatal(err)
	}
	if err := s.SetAlbum(ctx, "streambeats-lofi-gold"); err != nil {
		t.Fatal(err)
	}
	// Age the selection's own stamp: what's under test is two advances against
	// each other, not an advance against the switch that started the album.
	s.mu.Lock()
	s.lastStart = time.Time{}
	s.mu.Unlock()

	for range 2 {
		if err := s.Advance(ctx); err != nil {
			t.Fatal(err)
		}
	}
	// Sequential order over "a track.mp3".."d track.mp3": one advance off the
	// first track lands on the second, and the duplicate must not reach the third.
	if got := filepath.Base(o.file); got != "b track.mp3" {
		t.Errorf("two reports of one ending played %q, want b track.mp3", got)
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
	if o.file != CarHumFile {
		t.Fatalf("advance moved a non-album bed: %s", o.file)
	}
}

func TestDetect_ReadsTheLiveBedFromOBS(t *testing.T) {
	dir := shareDir(t, 1)
	for _, tc := range []struct {
		name     string
		settings map[string]any
		want     Bed
	}{
		{"network stream", map[string]any{"is_local_file": false}, SomaFM},
		{"carhum flac", map[string]any{"is_local_file": true, "local_file": CarHumFile}, CarHum},
		// The watchdog's copy of the same drone means SomaFM is still the
		// selected bed, mid-outage. Reading it as CarHum switches the outage
		// machinery off and strands the stream on the drone after SomaFM
		// recovers — the restart-during-fallback case.
		{"watchdog fallback flac", map[string]any{"is_local_file": true, "local_file": FallbackFile}, SomaFM},
		{"album track", map[string]any{"is_local_file": true, "local_file": filepath.Join(dir, "fifty-horizons", "a track.mp3")}, Album},
		// A sibling directory whose name merely prefixes the share must not
		// read as the album.
		{"lookalike dir", map[string]any{"is_local_file": true, "local_file": dir + "-old/x.mp3"}, CarHum},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := NewStore(&fakeOBS{settings: tc.settings}, SomaFM, dir, "twitch")
			s.Detect(context.Background())
			if bed, _ := s.Current(); bed != tc.want {
				t.Fatalf("detected %s, want %s", bed, tc.want)
			}
		})
	}
}

// Detecting the album has to build the play order too. OBS picks the boot track
// itself when the album is a platform's default bed, so no Set runs — and
// without a play order Advance has nowhere to go, leaving the stream silent
// after that one track ends.
func TestDetect_AlbumBuildsThePlayOrder(t *testing.T) {
	dir := shareDir(t, 3)
	playing := filepath.Join(dir, "fifty-horizons", "a track.mp3")
	o := &fakeOBS{settings: map[string]any{"is_local_file": true, "local_file": playing}}
	s := NewStore(o, CarHum, dir, "twitch")
	s.Detect(context.Background())

	if bed, track := s.Current(); bed != Album || track != playing {
		t.Fatalf("Current() = %s, %q; want album playing %q", bed, track, playing)
	}
	if err := s.Advance(context.Background()); err != nil {
		t.Fatal(err)
	}
	if o.file == "" || o.file == playing {
		t.Fatalf("advance wrote %q; want a different track", o.file)
	}
}

// An album detected with no tracks to scan leaves the bed reported honestly
// rather than crashing or claiming a track — the console shows what's on air.
func TestDetect_AlbumWithNoTracksStillReportsTheBed(t *testing.T) {
	dir := t.TempDir()
	o := &fakeOBS{settings: map[string]any{
		"is_local_file": true,
		"local_file":    filepath.Join(dir, "gone", "x.mp3"),
	}}
	s := NewStore(o, CarHum, dir, "twitch")
	s.Detect(context.Background())
	if bed, track := s.Current(); bed != Album || track != "" {
		t.Fatalf("Current() = %s, %q; want album with no track", bed, track)
	}
}

// A tripbot restart must not report the default channel while OBS plays another
// one — the scene's `input` URL is the only record of which is tuned.
func TestDetect_ReadsTheStationFromTheSourceURL(t *testing.T) {
	o := &fakeOBS{settings: map[string]any{
		"is_local_file": false,
		"input":         "https://ice4.somafm.com/spacestation-128-mp3",
	}}
	s := NewStore(o, CarHum, t.TempDir(), "twitch")
	s.Detect(context.Background())
	if s.Station() != "spacestation" {
		t.Fatalf("station = %q, want spacestation", s.Station())
	}
}

func TestDetect_KeepsTheSeedWhenOBSIsUnreachable(t *testing.T) {
	s := NewStore(&fakeOBS{err: errors.New("nope")}, SomaFM, t.TempDir(), "twitch")
	s.Detect(context.Background())
	if bed, _ := s.Current(); bed != SomaFM {
		t.Fatalf("want the seed bed retained, got %s", bed)
	}
}

// Wrapping the play order reshuffles it, and a plain shuffle can deal the track
// that just finished straight back to the front — an immediate repeat is the
// one ordering a listener notices. Walk several full wraps so the reshuffle is
// exercised many times rather than once.
func TestAdvance_NeverRepeatsATrackAcrossTheWrap(t *testing.T) {
	o := &fakeOBS{}
	s := NewStore(o, CarHum, shareDir(t, 3), "twitch")
	if err := s.Set(context.Background(), Album); err != nil {
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
	s := NewStore(o, Album, shareDir(t, 3), "twitch")
	ctx := context.Background()
	if err := s.Set(ctx, SomaFM); err != nil {
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
		t.Errorf("the superseded somafm switch reached OBS: url=%q", o.url)
	}
	if o.file != CarHumFile {
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
	if err := s.SetStation(context.Background(), "dronezone"); err != nil {
		t.Fatal(err)
	}

	sw, ok := s.Pending()
	if !ok {
		t.Fatal("want the tune reported as pending")
	}
	if sw.Bed != SomaFM || sw.Station != "dronezone" {
		t.Errorf("pending = %+v, want the somafm bed on dronezone", sw)
	}
	if sw.At.Before(time.Now()) {
		t.Errorf("pending lands at %v, already in the past", sw.At)
	}
	if got := s.Station(); got == "dronezone" {
		t.Error("the pending station is being reported as the live one")
	}

	waitFor(t, "the tune to land", func() bool {
		_, pending := s.Pending()
		return !pending
	})
	if got := s.Station(); got != "dronezone" {
		t.Errorf("station = %q after the tune landed, want dronezone", got)
	}
	if bed, _ := s.Current(); bed != SomaFM {
		t.Errorf("bed = %q after the tune landed, want somafm", bed)
	}
}
