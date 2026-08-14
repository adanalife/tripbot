package viewstats

import (
	"context"
	"testing"
	"time"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/database/testdb"
	"gorm.io/gorm"
)

// testConf is the config the writers under test read: a fixed platform and
// ReadOnly false so writes aren't skipped. The read-only tests pass their own.
var testConf = &c.TripbotConfig{Environment: "testing", Platform: "twitch"}

// setup installs a transaction-scoped DB and clears the package-global
// current-video tag and open-play tracking so each test starts from a known
// state. The transaction rolls back in cleanup, so rows never leak.
func setup(t *testing.T) *gorm.DB {
	t.Helper()
	db := testdb.New(t)

	currentVideoID.Store(0)
	resetOpenPlays()
	t.Cleanup(func() {
		currentVideoID.Store(0)
		resetOpenPlays()
	})
	return db
}

func resetOpenPlays() {
	openPlayMu.Lock()
	openPlayIDs = map[string]int{}
	openPlayMu.Unlock()
}

func allPlays(t *testing.T, db *gorm.DB) []VideoPlay {
	t.Helper()
	var plays []VideoPlay
	if err := db.Order("id").Find(&plays).Error; err != nil {
		t.Fatalf("read video_plays: %v", err)
	}
	return plays
}

func allSamples(t *testing.T, db *gorm.DB) []ViewerSample {
	t.Helper()
	var samples []ViewerSample
	if err := db.Order("id").Find(&samples).Error; err != nil {
		t.Fatalf("read viewer_samples: %v", err)
	}
	return samples
}

func TestRecordPlay_PersistsDenormalizedColumns(t *testing.T) {
	db := setup(t)

	RecordPlay(context.Background(), testConf, 42, "Utah", true, 38.5, -109.5)

	plays := allPlays(t, db)
	if len(plays) != 1 {
		t.Fatalf("expected 1 video_plays row, got %d", len(plays))
	}
	got := plays[0]
	if got.Platform != "twitch" {
		t.Errorf("platform: want twitch, got %q", got.Platform)
	}
	if got.VideoID == nil || *got.VideoID != 42 {
		t.Errorf("video_id: want 42, got %v", got.VideoID)
	}
	if got.State != "Utah" || !got.Flagged {
		t.Errorf("state/flagged: want Utah/true, got %q/%v", got.State, got.Flagged)
	}
	if got.Lat != 38.5 || got.Lng != -109.5 {
		t.Errorf("lat/lng: want 38.5/-109.5, got %v/%v", got.Lat, got.Lng)
	}
	// autoCreateTime must stamp started_at rather than writing the zero value
	// over its DEFAULT CURRENT_TIMESTAMP (the pkg/events regression).
	if time.Since(got.StartedAt) > time.Minute {
		t.Errorf("started_at not stamped at insert: %v", got.StartedAt)
	}
}

// A clip with no DB row (LoadOrCreate failed) still records the switch, with a
// NULL video_id.
func TestRecordPlay_ZeroVideoIDWritesNull(t *testing.T) {
	db := setup(t)

	RecordPlay(context.Background(), testConf, 0, "", false, 0, 0)

	plays := allPlays(t, db)
	if len(plays) != 1 {
		t.Fatalf("expected 1 video_plays row, got %d", len(plays))
	}
	if plays[0].VideoID != nil {
		t.Errorf("video_id: want NULL, got %v", *plays[0].VideoID)
	}
}

// A new play closes the previous one: the old clip stopped airing the moment
// the new one started, so its row gets an ended_at while the new row stays
// open. The first play of a process closes nothing.
func TestRecordPlay_ClosesPreviousPlay(t *testing.T) {
	db := setup(t)
	ctx := context.Background()

	RecordPlay(ctx, testConf, 42, "Utah", false, 38.5, -109.5)
	RecordPlay(ctx, testConf, 43, "Colorado", false, 39.1, -108.5)

	plays := allPlays(t, db)
	if len(plays) != 2 {
		t.Fatalf("expected 2 video_plays rows, got %d", len(plays))
	}
	first, second := plays[0], plays[1]
	if first.EndedAt == nil {
		t.Fatal("first play not closed by the second")
	}
	if first.EndedAt.Before(first.StartedAt) {
		t.Errorf("ended_at %v precedes started_at %v", first.EndedAt, first.StartedAt)
	}
	if second.EndedAt != nil {
		t.Errorf("second (current) play should stay open, got ended_at %v", *second.EndedAt)
	}
}

// A row left open by a previous process stays open: its clip's end was never
// observed, so the first play after a restart must not back-date an ended_at
// onto it.
func TestRecordPlay_LeavesPriorProcessRowOpen(t *testing.T) {
	db := setup(t)
	ctx := context.Background()

	// A row from a previous process: present in the table, absent from this
	// process's open-play tracking.
	orphanID := 42
	orphan := VideoPlay{Platform: "twitch", VideoID: &orphanID, State: "Utah"}
	if err := db.Create(&orphan).Error; err != nil {
		t.Fatalf("seed orphan play: %v", err)
	}
	resetOpenPlays()

	RecordPlay(ctx, testConf, 43, "Colorado", false, 39.1, -108.5)

	plays := allPlays(t, db)
	if len(plays) != 2 {
		t.Fatalf("expected 2 video_plays rows, got %d", len(plays))
	}
	if plays[0].EndedAt != nil {
		t.Errorf("prior process's row must stay open, got ended_at %v", *plays[0].EndedAt)
	}
}

// Closing is per-platform: platform A's clip switch says nothing about what B
// is airing, so B's open row must survive A's new play.
func TestRecordPlay_ClosesOnlyOwnPlatform(t *testing.T) {
	db := setup(t)
	ctx := context.Background()
	youtubeConf := &c.TripbotConfig{Environment: "testing", Platform: "youtube"}

	RecordPlay(ctx, testConf, 42, "Utah", false, 38.5, -109.5)
	RecordPlay(ctx, youtubeConf, 43, "Colorado", false, 39.1, -108.5)

	plays := allPlays(t, db)
	if len(plays) != 2 {
		t.Fatalf("expected 2 video_plays rows, got %d", len(plays))
	}
	for i, p := range plays {
		if p.EndedAt != nil {
			t.Errorf("play %d (%s) should stay open, got ended_at %v", i, p.Platform, *p.EndedAt)
		}
	}
}

func TestRecordSample_TagsCurrentVideo(t *testing.T) {
	db := setup(t)
	ctx := context.Background()

	// Before any play, the sample carries a NULL video_id.
	RecordSample(ctx, testConf, 3)
	// A play tags every sample that follows it.
	RecordPlay(ctx, testConf, 42, "Utah", false, 38.5, -109.5)
	RecordSample(ctx, testConf, 5)
	// A play with no DB row resets the tag back to NULL.
	RecordPlay(ctx, testConf, 0, "", true, 0, 0)
	RecordSample(ctx, testConf, 7)

	samples := allSamples(t, db)
	if len(samples) != 3 {
		t.Fatalf("expected 3 viewer_samples rows, got %d", len(samples))
	}

	wantCounts := []int{3, 5, 7}
	wantVideo := []*int{nil, intPtr(42), nil}
	for i, got := range samples {
		if got.Platform != "twitch" {
			t.Errorf("sample %d platform: want twitch, got %q", i, got.Platform)
		}
		if got.Count != wantCounts[i] {
			t.Errorf("sample %d count: want %d, got %d", i, wantCounts[i], got.Count)
		}
		switch {
		case wantVideo[i] == nil && got.VideoID != nil:
			t.Errorf("sample %d video_id: want NULL, got %d", i, *got.VideoID)
		case wantVideo[i] != nil && (got.VideoID == nil || *got.VideoID != *wantVideo[i]):
			t.Errorf("sample %d video_id: want %d, got %v", i, *wantVideo[i], got.VideoID)
		}
		if time.Since(got.SampledAt) > time.Minute {
			t.Errorf("sample %d sampled_at not stamped at insert: %v", i, got.SampledAt)
		}
	}
}

func TestReadOnly_SkipsWritesButStillTracksVideo(t *testing.T) {
	db := setup(t)
	readOnlyConf := &c.TripbotConfig{Environment: "testing", Platform: "twitch", ReadOnly: true}
	ctx := context.Background()

	RecordPlay(ctx, readOnlyConf, 42, "Utah", false, 38.5, -109.5)
	RecordSample(ctx, readOnlyConf, 5)

	if plays := allPlays(t, db); len(plays) != 0 {
		t.Errorf("expected no video_plays rows in read-only mode, got %d", len(plays))
	}
	if samples := allSamples(t, db); len(samples) != 0 {
		t.Errorf("expected no viewer_samples rows in read-only mode, got %d", len(samples))
	}
	// The tag is stored before the read-only bail, so writes resume correctly
	// tagged the moment a non-read-only config is used.
	RecordSample(ctx, testConf, 5)
	samples := allSamples(t, db)
	if len(samples) != 1 {
		t.Fatalf("expected 1 viewer_samples row, got %d", len(samples))
	}
	if samples[0].VideoID == nil || *samples[0].VideoID != 42 {
		t.Errorf("video_id: want 42, got %v", samples[0].VideoID)
	}
}

func intPtr(i int) *int { return &i }
