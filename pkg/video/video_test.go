package video

import (
	"context"
	"database/sql"
	"fmt"
	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"testing"
	"time"

	"github.com/adanalife/tripbot/pkg/database/testdb"
	"gorm.io/gorm"
)

// These tests cover the *Player state-machine introduced in #600. The Player is
// constructable, so it can be pointed at an httptest-backed playout client, a
// recording onscreens fake, and a real postgres transaction — each transition
// loads the clip and records its play against the actual schema.
//
// GetCurrentlyPlaying reads the fake playout server on every platform, so these
// assertions hold identically wherever `go test` runs.

// testConf is the config test Players carry — env + platform for event tags,
// ReadOnly false so the video_plays writes aren't skipped.
var testConf = &c.TripbotConfig{Environment: "testing", Platform: "twitch"}

// crossings returns the state_crossing rows written so far, oldest first.
// VideoTsSec is nil where the writer only knew the clip.
func crossings(t *testing.T, db *gorm.DB) []crossingRow {
	t.Helper()
	var rows []crossingRow
	if err := db.Raw(`SELECT meta->>'from' AS from_state, meta->>'to' AS to_state,
		(meta->>'sequential')::bool AS sequential, video_id, video_ts_sec
		FROM events WHERE event = 'state_crossing' ORDER BY id`).Scan(&rows).Error; err != nil {
		t.Fatalf("read state_crossing events: %v", err)
	}
	return rows
}

type crossingRow struct {
	FromState, ToState string
	Sequential         bool
	VideoID            *int
	VideoTsSec         *float64
}

// crossed renders a row the way the assertions read it.
func (r crossingRow) crossed() string {
	return fmt.Sprintf("%s->%s sequential=%v", r.FromState, r.ToState, r.Sequential)
}

// insertTrack writes a dense OCR track for vid: one row every 2 s from 0 to
// through, in state `state` up to and including crossAt and in `then` after.
func insertTrack(t *testing.T, db *gorm.DB, vid Video, through int, state string, crossAt int, then string) {
	t.Helper()
	for ts := 0; ts <= through; ts += 2 {
		st := state
		if ts > crossAt {
			st = then
		}
		if err := db.Exec(`INSERT INTO video_coords (video_id, ts_sec, source_ts_sec, lat, lng, source, state)
			VALUES (?, ?, ?, 40, -110, 'ocr', ?)`, vid.ID, ts, ts, st).Error; err != nil {
			t.Fatalf("insert coord: %v", err)
		}
	}
}

// airingPlayer returns a Player whose Playhead answers from its own clock —
// there is no JetStream in a test, so Playhead falls back to CurrentlyPlaying
// + CurrentProgress — showing vid at `at` into it.
func airingPlayer(t *testing.T, vid Video, at time.Duration) *Player {
	t.Helper()
	playoutCurrent := ""
	p := NewPlayer(testConf, &recordingOnscreens{}, fakePlayoutServer(t, &playoutCurrent))
	p.show(vid, at)
	return p
}

func (p *Player) show(vid Video, at time.Duration) {
	p.CurrentlyPlaying = vid
	p.timeStarted = time.Now().Add(-at)
}

func TestTrackState_MidClipCrossingLandsAtTheMoment(t *testing.T) {
	db := testdb.New(t)
	conf := 1.0
	vid := insertVideo(t, db, Video{Slug: "2018_0801_120000_001", State: "Utah", CoordConfidence: &conf})
	// Utah for the first 61 s of the track, Colorado from 62 s on.
	insertTrack(t, db, vid, 180, "Utah", 60, "Colorado")

	p := airingPlayer(t, vid, 10*time.Second)
	p.TrackState(context.Background())
	if got := crossings(t, db); len(got) != 0 {
		t.Fatalf("first observation wrote %d crossings, want none", len(got))
	}

	p.show(vid, 50*time.Second)
	p.TrackState(context.Background())
	if got := crossings(t, db); len(got) != 0 {
		t.Fatalf("still in Utah wrote %d crossings, want none", len(got))
	}

	p.show(vid, 90*time.Second)
	p.TrackState(context.Background())
	got := crossings(t, db)
	if len(got) != 1 {
		t.Fatalf("crossing into Colorado wrote %d rows, want 1", len(got))
	}
	if want := "Utah->Colorado sequential=true"; got[0].crossed() != want {
		t.Errorf("crossing = %s, want %s", got[0].crossed(), want)
	}
	if got[0].VideoID == nil || *got[0].VideoID != vid.ID {
		t.Errorf("video_id = %v, want %d", got[0].VideoID, vid.ID)
	}
	if got[0].VideoTsSec == nil || *got[0].VideoTsSec < 89 || *got[0].VideoTsSec > 92 {
		t.Errorf("video_ts_sec = %v, want ~90 (the playhead when the crossing was seen)", got[0].VideoTsSec)
	}

	// Colorado from here on: no second row.
	p.show(vid, 120*time.Second)
	p.TrackState(context.Background())
	if got := crossings(t, db); len(got) != 1 {
		t.Errorf("staying in Colorado wrote %d rows total, want 1", len(got))
	}
}

// A clip without a track answers at clip level, as the switch-time comparison
// did: sequential when the new clip is the previous one's next_vid, a jump
// otherwise, and no offset recorded because the moment isn't known.
func TestTrackState_ClipLevelFallback(t *testing.T) {
	db := testdb.New(t)
	colorado := insertVideo(t, db, Video{Slug: "2018_0801_120300_002", State: "Colorado"})
	kansas := insertVideo(t, db, Video{Slug: "2018_0801_120600_003", State: "Kansas"})
	utah := insertVideo(t, db, Video{Slug: "2018_0801_120000_001", State: "Utah",
		NextVid: sql.NullInt64{Int64: int64(colorado.ID), Valid: true}})

	p := airingPlayer(t, utah, 30*time.Second)
	p.TrackState(context.Background())
	p.show(colorado, 5*time.Second) // the clip that follows in corpus order
	p.TrackState(context.Background())
	p.show(kansas, 5*time.Second) // not colorado's next_vid: a jump
	p.TrackState(context.Background())

	got := crossings(t, db)
	if len(got) != 2 {
		t.Fatalf("wrote %d crossings, want 2", len(got))
	}
	if want := "Utah->Colorado sequential=true"; got[0].crossed() != want {
		t.Errorf("first crossing = %s, want %s", got[0].crossed(), want)
	}
	if want := "Colorado->Kansas sequential=false"; got[1].crossed() != want {
		t.Errorf("second crossing = %s, want %s", got[1].crossed(), want)
	}
	for i, row := range got {
		if row.VideoTsSec != nil {
			t.Errorf("row %d video_ts_sec = %v, want NULL for a clip-level crossing", i, *row.VideoTsSec)
		}
	}
}

// An unresolvable state is not a crossing in either direction: nothing to
// cross from, and nothing to cross into.
func TestTrackState_UnresolvableStateIsNotACrossing(t *testing.T) {
	db := testdb.New(t)
	utah := insertVideo(t, db, Video{Slug: "2018_0801_120000_001", State: "Utah"})
	blank := insertVideo(t, db, Video{Slug: "2018_0801_120300_002"})

	p := airingPlayer(t, blank, 5*time.Second)
	p.TrackState(context.Background())
	p.show(utah, 5*time.Second)
	p.TrackState(context.Background())
	p.show(blank, 5*time.Second)
	p.TrackState(context.Background())

	if got := crossings(t, db); len(got) != 0 {
		t.Errorf("wrote %d crossings, want none", len(got))
	}
}

func TestPlayer_Current_ZeroBeforeAnyCall(t *testing.T) {
	rec := &recordingOnscreens{}
	playoutCurrent := ""
	playout := fakePlayoutServer(t, &playoutCurrent)
	p := NewPlayer(testConf, rec, playout)

	got := p.Current()
	if got != (Video{}) {
		t.Errorf("Current() before any GetCurrentlyPlaying call = %+v, want zero Video", got)
	}
}

func TestPlayer_GetCurrentlyPlaying_FirstCall_FlaggedShowsGPS(t *testing.T) {
	db := testdb.New(t)
	rec := &recordingOnscreens{}
	playoutCurrent := "2018_0514_224801_013.MP4"
	playout := fakePlayoutServer(t, &playoutCurrent)
	p := NewPlayer(testConf, rec, playout)

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
	db := testdb.New(t)
	rec := &recordingOnscreens{}
	playoutCurrent := "2019_0615_183000_001.MP4"
	playout := fakePlayoutServer(t, &playoutCurrent)
	p := NewPlayer(testConf, rec, playout)

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
	db := testdb.New(t)
	rec := &recordingOnscreens{}
	playoutCurrent := "2018_0514_224801_013.MP4"
	playout := fakePlayoutServer(t, &playoutCurrent)
	p := NewPlayer(testConf, rec, playout)

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
	db := testdb.New(t)
	rec := &recordingOnscreens{}
	playoutCurrent := "2018_0514_224801_013.MP4"
	playout := fakePlayoutServer(t, &playoutCurrent)
	p := NewPlayer(testConf, rec, playout)

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
	db := testdb.New(t)
	rec := &recordingOnscreens{}
	playoutCurrent := "2018_0514_224801_013.MP4"
	playout := fakePlayoutServer(t, &playoutCurrent)
	p := NewPlayer(testConf, rec, playout)

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
	// When playout returns "" on the very first call, curVid stays "" and equals
	// preVid (also ""), so LoadOrCreate is never invoked and no play is recorded —
	// that absence is what the assertions below check.
	//
	// The fixture is taken purely for its gate: pkg/database log.Fatalf's the whole
	// test binary when DB config is absent, so a bare host needs the same
	// skip-when-postgres-is-unreachable treatment as the tests around it.
	testdb.New(t)

	rec := &recordingOnscreens{}
	playoutCurrent := ""
	playout := fakePlayoutServer(t, &playoutCurrent)
	p := NewPlayer(testConf, rec, playout)

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
