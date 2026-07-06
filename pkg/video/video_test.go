package video

import (
	"context"
	"runtime"
	"testing"
	"time"
)

// These tests cover the *Player state-machine introduced in #600. Before the
// refactor, GetCurrentlyPlaying was a package-level function with package-level
// state — no way to exercise the transition / GPS-overlay-toggle behaviour
// without standing up real HTTP + DB. Now the Player is constructable, so we
// can point it at httptest-backed clients and a sqlmock-backed gorm.DB.
//
// Darwin path (figureOutCurrentVideo via lsof script) is not exercised here;
// the Linux path is what runs in production and what CI tests against. The
// non-Darwin guard on the relevant tests keeps the assertions stable when
// running `go test` locally on a Mac.

// skipIfDarwin no-ops the test when GOOS=darwin. GetCurrentlyPlaying picks
// the figureOutCurrentVideo path on Darwin (lsof + bin/current-file.sh),
// bypassing the vlc client entirely — so the fake vlc server's response
// would be ignored and the test would shell out for real.
func skipIfDarwin(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "darwin" {
		t.Skip("GetCurrentlyPlaying takes the lsof path on darwin; covered in CI (linux)")
	}
}

func TestPlayer_Current_ZeroBeforeAnyCall(t *testing.T) {
	rec := &recordingOnscreens{}
	vlcCurrent := ""
	vlc := fakeVLCServer(t, &vlcCurrent)
	p := NewPlayer(rec, vlc)

	got := p.Current()
	if got != (Video{}) {
		t.Errorf("Current() before any GetCurrentlyPlaying call = %+v, want zero Video", got)
	}
}

func TestPlayer_GetCurrentlyPlaying_FirstCall_FlaggedShowsGPS(t *testing.T) {
	skipIfDarwin(t)
	mock := installMockDB(t)
	rec := &recordingOnscreens{}
	vlcCurrent := "2018_0514_224801_013.MP4"
	vlc := fakeVLCServer(t, &vlcCurrent)
	p := NewPlayer(rec, vlc)

	// DB returns a flagged video for the slug derived from vlcCurrent.
	expectLoadHit(mock, 1, "2018_0514_224801_013", true)
	expectPlayInsert(mock, 1, true)

	p.GetCurrentlyPlaying(context.Background())

	if p.Current().Slug != "2018_0514_224801_013" {
		t.Errorf("CurrentlyPlaying.Slug = %q, want %q", p.Current().Slug, "2018_0514_224801_013")
	}
	if !p.Current().Flagged {
		t.Error("expected CurrentlyPlaying.Flagged = true (staged in mock rows)")
	}
	if len(rec.calls) != 1 || rec.calls[0] != "ShowGPSImage" {
		t.Errorf("expected single ShowGPSImage call, got %v", rec.calls)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestPlayer_GetCurrentlyPlaying_FirstCall_NotFlaggedHidesGPS(t *testing.T) {
	skipIfDarwin(t)
	mock := installMockDB(t)
	rec := &recordingOnscreens{}
	vlcCurrent := "2019_0615_183000_001.MP4"
	vlc := fakeVLCServer(t, &vlcCurrent)
	p := NewPlayer(rec, vlc)

	expectLoadHit(mock, 7, "2019_0615_183000_001", false)
	expectPlayInsert(mock, 7, false)

	p.GetCurrentlyPlaying(context.Background())

	if p.Current().Flagged {
		t.Error("expected CurrentlyPlaying.Flagged = false (staged in mock rows)")
	}
	if len(rec.calls) != 1 || rec.calls[0] != "HideGPSImage" {
		t.Errorf("expected single HideGPSImage call, got %v", rec.calls)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestPlayer_GetCurrentlyPlaying_SameVidIsNoop(t *testing.T) {
	skipIfDarwin(t)
	mock := installMockDB(t)
	rec := &recordingOnscreens{}
	vlcCurrent := "2018_0514_224801_013.MP4"
	vlc := fakeVLCServer(t, &vlcCurrent)
	p := NewPlayer(rec, vlc)

	// Only one DB hit + play insert expected — the second GetCurrentlyPlaying
	// call sees curVid == preVid and short-circuits before reaching
	// LoadOrCreate.
	expectLoadHit(mock, 1, "2018_0514_224801_013", false)
	expectPlayInsert(mock, 1, false)

	p.GetCurrentlyPlaying(context.Background())
	timeStartedAfterFirst := p.timeStarted

	// Tiny sleep so a stray timeStarted reset would be observable.
	time.Sleep(2 * time.Millisecond)

	p.GetCurrentlyPlaying(context.Background())

	if p.timeStarted != timeStartedAfterFirst {
		t.Error("expected timeStarted unchanged across same-vid calls; got a reset")
	}
	if len(rec.calls) != 1 {
		t.Errorf("expected exactly one onscreens call (from first transition), got %d: %v", len(rec.calls), rec.calls)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestPlayer_GetCurrentlyPlaying_TransitionTogglesGPSAndResetsTimeStarted(t *testing.T) {
	skipIfDarwin(t)
	mock := installMockDB(t)
	rec := &recordingOnscreens{}
	vlcCurrent := "2018_0514_224801_013.MP4"
	vlc := fakeVLCServer(t, &vlcCurrent)
	p := NewPlayer(rec, vlc)

	// First vid: flagged → ShowGPSImage. Each transition loads the video then
	// records its play, so the expectations interleave.
	expectLoadHit(mock, 1, "2018_0514_224801_013", true)
	expectPlayInsert(mock, 1, true)
	// Second vid: not flagged → HideGPSImage.
	expectLoadHit(mock, 2, "2019_0615_183000_001", false)
	expectPlayInsert(mock, 2, false)

	p.GetCurrentlyPlaying(context.Background())
	firstStart := p.timeStarted
	firstSlug := p.Current().Slug

	// Flip the vlc-server's reported path; sleep so any reset to timeStarted
	// is observable as a strictly-greater value.
	vlcCurrent = "2019_0615_183000_001.MP4"
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
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestPlayer_CurrentProgress_TracksTimeSinceStart(t *testing.T) {
	skipIfDarwin(t)
	mock := installMockDB(t)
	rec := &recordingOnscreens{}
	vlcCurrent := "2018_0514_224801_013.MP4"
	vlc := fakeVLCServer(t, &vlcCurrent)
	p := NewPlayer(rec, vlc)

	expectLoadHit(mock, 1, "2018_0514_224801_013", false)
	expectPlayInsert(mock, 1, false)

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

func TestPlayer_GetCurrentlyPlaying_EmptyVlcResult_NoTransition(t *testing.T) {
	skipIfDarwin(t)
	// No mockDB needed — when vlc returns "" on the very first call,
	// curVid stays "" and equals preVid (also ""), so LoadOrCreate is
	// never invoked. installMockDB-less SetGormDB is left at nil; any
	// DB hit would NPE and fail the test loudly.
	rec := &recordingOnscreens{}
	vlcCurrent := ""
	vlc := fakeVLCServer(t, &vlcCurrent)
	p := NewPlayer(rec, vlc)

	p.GetCurrentlyPlaying(context.Background())

	if p.curVid != "" || p.preVid != "" {
		t.Errorf("curVid/preVid after empty-vlc first call = (%q, %q); want (\"\",\"\")", p.curVid, p.preVid)
	}
	if len(rec.calls) != 0 {
		t.Errorf("expected no overlay calls on no-transition path, got %v", rec.calls)
	}
	if p.Current() != (Video{}) {
		t.Errorf("Current() after empty-vlc first call = %+v, want zero Video", p.Current())
	}
}
