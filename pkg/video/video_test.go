package video

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/adanalife/tripbot/pkg/database/testdb"
)

// These tests cover the *Player state-machine introduced in #600. The Player is
// constructable, so it can be pointed at an httptest-backed playout client, a
// recording onscreens fake, and a real postgres transaction — each transition
// loads the clip and records its play against the actual schema.
//
// Darwin path (figureOutCurrentVideo via lsof script) is not exercised here;
// the Linux path is what runs in production and what CI tests against. The
// non-Darwin guard on the relevant tests keeps the assertions stable when
// running `go test` locally on a Mac.

// skipIfDarwin no-ops the test when GOOS=darwin. GetCurrentlyPlaying picks
// the figureOutCurrentVideo path on Darwin (lsof + bin/current-file.sh),
// bypassing the playout client entirely — so the fake playout server's response
// would be ignored and the test would shell out for real.
func skipIfDarwin(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "darwin" {
		t.Skip("GetCurrentlyPlaying takes the lsof path on darwin; covered in CI (linux)")
	}
}

func TestPlayer_Current_ZeroBeforeAnyCall(t *testing.T) {
	rec := &recordingOnscreens{}
	playoutCurrent := ""
	playout := fakePlayoutServer(t, &playoutCurrent)
	p := NewPlayer(rec, playout)

	got := p.Current()
	if got != (Video{}) {
		t.Errorf("Current() before any GetCurrentlyPlaying call = %+v, want zero Video", got)
	}
}

func TestPlayer_GetCurrentlyPlaying_FirstCall_FlaggedShowsGPS(t *testing.T) {
	skipIfDarwin(t)
	db := testdb.New(t)
	rec := &recordingOnscreens{}
	playoutCurrent := "2018_0514_224801_013.MP4"
	playout := fakePlayoutServer(t, &playoutCurrent)
	p := NewPlayer(rec, playout)

	// The clip playout reports is already in the DB, flagged (no GPS fix).
	vid := insertVideo(t, db, Video{Slug: "2018_0514_224801_013", Flagged: true, CoordSource: CoordSourceMissing})

	p.GetCurrentlyPlaying(context.Background())

	if p.Current().ID != vid.ID {
		t.Errorf("CurrentlyPlaying.ID = %d, want the persisted %d", p.Current().ID, vid.ID)
	}
	if p.Current().Slug != "2018_0514_224801_013" {
		t.Errorf("CurrentlyPlaying.Slug = %q, want %q", p.Current().Slug, "2018_0514_224801_013")
	}
	if !p.Current().Flagged {
		t.Error("expected CurrentlyPlaying.Flagged = true (the row is flagged)")
	}
	if len(rec.calls) != 1 || rec.calls[0] != "ShowGPSImage" {
		t.Errorf("expected single ShowGPSImage call, got %v", rec.calls)
	}
	// The transition is durable: a video_plays row lands with the clip's state.
	if n := playCount(t, db, vid.ID); n != 1 {
		t.Errorf("video_plays rows for video %d = %d, want 1", vid.ID, n)
	}
	var flagged bool
	if err := db.Raw(`SELECT flagged FROM video_plays WHERE video_id = ?`, vid.ID).Scan(&flagged).Error; err != nil {
		t.Fatalf("read video_plays.flagged: %v", err)
	}
	if !flagged {
		t.Error("video_plays row recorded flagged = false for a flagged clip")
	}
}

func TestPlayer_GetCurrentlyPlaying_FirstCall_NotFlaggedHidesGPS(t *testing.T) {
	skipIfDarwin(t)
	db := testdb.New(t)
	rec := &recordingOnscreens{}
	playoutCurrent := "2019_0615_183000_001.MP4"
	playout := fakePlayoutServer(t, &playoutCurrent)
	p := NewPlayer(rec, playout)

	vid := insertVideo(t, db, Video{Slug: "2019_0615_183000_001", State: "Oregon", Lat: 45.5, Lng: -122.6})

	p.GetCurrentlyPlaying(context.Background())

	if p.Current().Flagged {
		t.Error("expected CurrentlyPlaying.Flagged = false (the row is unflagged)")
	}
	if p.Current().State != "Oregon" {
		t.Errorf("CurrentlyPlaying.State = %q, want %q", p.Current().State, "Oregon")
	}
	if len(rec.calls) != 1 || rec.calls[0] != "HideGPSImage" {
		t.Errorf("expected single HideGPSImage call, got %v", rec.calls)
	}
	if n := playCount(t, db, vid.ID); n != 1 {
		t.Errorf("video_plays rows for video %d = %d, want 1", vid.ID, n)
	}
}

func TestPlayer_GetCurrentlyPlaying_SameVidIsNoop(t *testing.T) {
	skipIfDarwin(t)
	db := testdb.New(t)
	rec := &recordingOnscreens{}
	playoutCurrent := "2018_0514_224801_013.MP4"
	playout := fakePlayoutServer(t, &playoutCurrent)
	p := NewPlayer(rec, playout)

	vid := insertVideo(t, db, Video{Slug: "2018_0514_224801_013", State: "Nevada", Lat: 39.5, Lng: -119.8})

	p.GetCurrentlyPlaying(context.Background())
	timeStartedAfterFirst := p.timeStarted

	// Tiny sleep so a stray timeStarted reset would be observable.
	time.Sleep(2 * time.Millisecond)

	// Second call sees curVid == preVid and short-circuits before LoadOrCreate,
	// so no second play is recorded.
	p.GetCurrentlyPlaying(context.Background())

	if p.timeStarted != timeStartedAfterFirst {
		t.Error("expected timeStarted unchanged across same-vid calls; got a reset")
	}
	if len(rec.calls) != 1 {
		t.Errorf("expected exactly one onscreens call (from first transition), got %d: %v", len(rec.calls), rec.calls)
	}
	if n := playCount(t, db, vid.ID); n != 1 {
		t.Errorf("video_plays rows for video %d = %d, want 1 (the no-op call recorded a play)", vid.ID, n)
	}
}

func TestPlayer_GetCurrentlyPlaying_TransitionTogglesGPSAndResetsTimeStarted(t *testing.T) {
	skipIfDarwin(t)
	db := testdb.New(t)
	rec := &recordingOnscreens{}
	playoutCurrent := "2018_0514_224801_013.MP4"
	playout := fakePlayoutServer(t, &playoutCurrent)
	p := NewPlayer(rec, playout)

	// First vid: flagged → ShowGPSImage. Second: unflagged → HideGPSImage.
	first := insertVideo(t, db, Video{Slug: "2018_0514_224801_013", Flagged: true, CoordSource: CoordSourceMissing})
	second := insertVideo(t, db, Video{Slug: "2019_0615_183000_001", State: "Oregon", Lat: 45.5, Lng: -122.6})

	p.GetCurrentlyPlaying(context.Background())
	firstStart := p.timeStarted
	firstSlug := p.Current().Slug

	// Flip the playout-server's reported path; sleep so any reset to timeStarted
	// is observable as a strictly-greater value.
	playoutCurrent = "2019_0615_183000_001.MP4"
	time.Sleep(2 * time.Millisecond)

	p.GetCurrentlyPlaying(context.Background())

	if p.preVid != "2018_0514_224801_013.MP4" {
		t.Errorf("preVid after transition = %q, want %q", p.preVid, "2018_0514_224801_013.MP4")
	}
	if p.curVid != "2019_0615_183000_001.MP4" {
		t.Errorf("curVid after transition = %q, want %q", p.curVid, "2019_0615_183000_001.MP4")
	}
	if !p.timeStarted.After(firstStart) {
		t.Error("expected timeStarted reset to a later instant on transition")
	}
	if p.Current().Slug == firstSlug {
		t.Errorf("CurrentlyPlaying.Slug unchanged after transition (still %q)", firstSlug)
	}
	wantOverlay := []string{"ShowGPSImage", "HideGPSImage"}
	if len(rec.calls) != len(wantOverlay) {
		t.Fatalf("expected overlay sequence %v, got %v", wantOverlay, rec.calls)
	}
	for i, want := range wantOverlay {
		if rec.calls[i] != want {
			t.Errorf("overlay call %d: want %q, got %q", i, want, rec.calls[i])
		}
	}
	// Each transition records its own play.
	if n := playCount(t, db, first.ID); n != 1 {
		t.Errorf("video_plays rows for the first clip = %d, want 1", n)
	}
	if n := playCount(t, db, second.ID); n != 1 {
		t.Errorf("video_plays rows for the second clip = %d, want 1", n)
	}
}

func TestPlayer_CurrentProgress_TracksTimeSinceStart(t *testing.T) {
	skipIfDarwin(t)
	db := testdb.New(t)
	rec := &recordingOnscreens{}
	playoutCurrent := "2018_0514_224801_013.MP4"
	playout := fakePlayoutServer(t, &playoutCurrent)
	p := NewPlayer(rec, playout)

	insertVideo(t, db, Video{Slug: "2018_0514_224801_013", State: "Nevada", Lat: 39.5, Lng: -119.8})

	p.GetCurrentlyPlaying(context.Background())
	time.Sleep(10 * time.Millisecond)

	got := p.CurrentProgress()
	if got < 5*time.Millisecond {
		t.Errorf("CurrentProgress() = %v, expected at least ~10ms since GetCurrentlyPlaying", got)
	}
	if got > time.Second {
		t.Errorf("CurrentProgress() = %v, suspiciously large for a 10ms sleep", got)
	}
}

func TestPlayer_GetCurrentlyPlaying_EmptyPlayoutResult_NoTransition(t *testing.T) {
	skipIfDarwin(t)
	// No testdb needed — when playout returns "" on the very first call, curVid
	// stays "" and equals preVid (also ""), so LoadOrCreate is never invoked.
	// The database singleton is left nil; any DB hit would panic and fail the
	// test loudly.
	rec := &recordingOnscreens{}
	playoutCurrent := ""
	playout := fakePlayoutServer(t, &playoutCurrent)
	p := NewPlayer(rec, playout)

	p.GetCurrentlyPlaying(context.Background())

	if p.curVid != "" || p.preVid != "" {
		t.Errorf("curVid/preVid after empty-playout first call = (%q, %q); want (\"\",\"\")", p.curVid, p.preVid)
	}
	if len(rec.calls) != 0 {
		t.Errorf("expected no overlay calls on no-transition path, got %v", rec.calls)
	}
	if p.Current() != (Video{}) {
		t.Errorf("Current() after empty-playout first call = %+v, want zero Video", p.Current())
	}
}
