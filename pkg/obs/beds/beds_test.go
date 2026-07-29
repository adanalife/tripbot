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
	file     string
	loop     bool
	settings map[string]any
	err      error
}

func (f *fakeOBS) SetNetwork(context.Context, string) error {
	if f.err != nil {
		return f.err
	}
	f.network, f.file = true, ""
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

// albumDir writes n dummy tracks (plus a non-audio file that must be ignored)
// and returns the directory.
func albumDir(t *testing.T, n int) string {
	t.Helper()
	dir := t.TempDir()
	for i := 0; i < n; i++ {
		name := filepath.Join(dir, string(rune('a'+i))+" track.mp3")
		if err := os.WriteFile(name, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "readme.txt"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestSet_SomaFMUsesNetworkMode(t *testing.T) {
	o := &fakeOBS{}
	s := NewStore(o, CarHum, albumDir(t, 2))
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

func TestSet_CarHumLoops(t *testing.T) {
	o := &fakeOBS{}
	s := NewStore(o, SomaFM, albumDir(t, 2))
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
	s := NewStore(o, CarHum, dir)
	if err := s.Set(context.Background(), Album); err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(o.file) != dir {
		t.Fatalf("album track %q not from %q", o.file, dir)
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
	dir := albumDir(t, 1) // one .mp3 + one readme.txt
	o := &fakeOBS{}
	s := NewStore(o, CarHum, dir)
	if err := s.Set(context.Background(), Album); err != nil {
		t.Fatal(err)
	}
	if filepath.Ext(o.file) != ".mp3" {
		t.Fatalf("picked a non-audio file: %s", o.file)
	}
}

func TestSet_AlbumWithNoTracksFails(t *testing.T) {
	o := &fakeOBS{}
	s := NewStore(o, CarHum, t.TempDir())
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
	s := NewStore(o, SomaFM, albumDir(t, 2))
	if err := s.Set(context.Background(), CarHum); err == nil {
		t.Fatal("want an error when OBS rejects the switch")
	}
	if bed, _ := s.Current(); bed != SomaFM {
		t.Fatalf("bed after a rejected switch: want somafm, got %s", bed)
	}
}

func TestSet_RejectsUnknownBed(t *testing.T) {
	s := NewStore(&fakeOBS{}, CarHum, t.TempDir())
	if err := s.Set(context.Background(), Bed("wurlitzer")); err == nil {
		t.Fatal("want an error for an unknown bed")
	}
}

func TestAdvance_WalksEveryTrackThenWraps(t *testing.T) {
	const n = 4
	o := &fakeOBS{}
	s := NewStore(o, CarHum, albumDir(t, n))
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
	s := NewStore(o, CarHum, albumDir(t, 3))
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
		{"album track", map[string]any{"is_local_file": true, "local_file": filepath.Join(dir, "a track.mp3")}, Album},
		// A sibling directory whose name merely prefixes the share must not
		// read as the album.
		{"lookalike dir", map[string]any{"is_local_file": true, "local_file": dir + "-old/x.mp3"}, CarHum},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := NewStore(&fakeOBS{settings: tc.settings}, SomaFM, dir)
			s.Detect(context.Background())
			if bed, _ := s.Current(); bed != tc.want {
				t.Fatalf("detected %s, want %s", bed, tc.want)
			}
		})
	}
}

func TestDetect_KeepsTheSeedWhenOBSIsUnreachable(t *testing.T) {
	s := NewStore(&fakeOBS{err: errors.New("nope")}, SomaFM, t.TempDir())
	s.Detect(context.Background())
	if bed, _ := s.Current(); bed != SomaFM {
		t.Fatalf("want the seed bed retained, got %s", bed)
	}
}
