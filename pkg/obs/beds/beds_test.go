package beds

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

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

// albumDir builds a share holding one album of n tracks. It mirrors the real
// layout: the album is a SUBDIRECTORY, and the share root also holds a loose
// audio file (the 556MB carsounds.m4a lives there) plus a non-audio file —
// neither of which is a track. Returns the share root, which is what gets
// mounted.
func albumDir(t *testing.T, n int) string {
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

func TestSet_SomaFMUsesNetworkMode(t *testing.T) {
	o := &fakeOBS{}
	s := NewStore(o, CarHum, albumDir(t, 2), "twitch")
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
	s := NewStore(o, CarHum, albumDir(t, 2), "twitch")
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
	s := NewStore(o, SomaFM, albumDir(t, 2), "twitch")
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
	s := NewStore(o, SomaFM, albumDir(t, 2), "twitch")
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
	dir := albumDir(t, 3)
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
	dir := albumDir(t, 1)
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
	dir := albumDir(t, 3)
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
	s := NewStore(o, SomaFM, albumDir(t, 2), "twitch")
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
	s := NewStore(o, CarHum, albumDir(t, n), "twitch")
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

func TestAdvance_NoopOnOtherBeds(t *testing.T) {
	o := &fakeOBS{}
	s := NewStore(o, CarHum, albumDir(t, 3), "twitch")
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
	dir := albumDir(t, 1)
	for _, tc := range []struct {
		name     string
		settings map[string]any
		want     Bed
	}{
		{"network stream", map[string]any{"is_local_file": false}, SomaFM},
		{"carhum flac", map[string]any{"is_local_file": true, "local_file": CarHumFile}, CarHum},
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
	dir := albumDir(t, 3)
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
	s := NewStore(o, CarHum, albumDir(t, 3), "twitch")
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
