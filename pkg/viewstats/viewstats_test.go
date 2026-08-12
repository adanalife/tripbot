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

// setup installs a transaction-scoped DB, which rolls back in cleanup so rows
// never leak between tests.
func setup(t *testing.T) *gorm.DB {
	t.Helper()
	return testdb.New(t)
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

// A sample is tagged with the clip the caller says is on screen. It comes from
// the player, not from a remembered prior RecordPlay — so the very first sample
// after a restart, with no clip switch yet observed, still names its footage.
func TestRecordSample_TagsTheGivenVideo(t *testing.T) {
	db := setup(t)
	ctx := context.Background()

	RecordSample(ctx, testConf, 5, Audience{}, 42)
	// 0 means nothing playing, or a clip with no DB row: a NULL, not clip 0.
	RecordSample(ctx, testConf, 7, Audience{}, 0)

	samples := allSamples(t, db)
	if len(samples) != 2 {
		t.Fatalf("expected 2 viewer_samples rows, got %d", len(samples))
	}
	if samples[0].VideoID == nil || *samples[0].VideoID != 42 {
		t.Errorf("first sample video_id = %v, want 42", samples[0].VideoID)
	}
	if samples[1].VideoID != nil {
		t.Errorf("second sample video_id = %d, want NULL", *samples[1].VideoID)
	}
	for i, got := range samples {
		if got.Platform != "twitch" {
			t.Errorf("sample %d platform: want twitch, got %q", i, got.Platform)
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
	RecordSample(ctx, readOnlyConf, 5, Audience{}, 0)

	if plays := allPlays(t, db); len(plays) != 0 {
		t.Errorf("expected no video_plays rows in read-only mode, got %d", len(plays))
	}
	if samples := allSamples(t, db); len(samples) != 0 {
		t.Errorf("expected no viewer_samples rows in read-only mode, got %d", len(samples))
	}
	// The tag is stored before the read-only bail, so writes resume correctly
	// tagged the moment a non-read-only config is used.
	RecordSample(ctx, testConf, 5, Audience{}, 0)
	samples := allSamples(t, db)
	if len(samples) != 1 {
		t.Fatalf("expected 1 viewer_samples row, got %d", len(samples))
	}
	if samples[0].VideoID == nil || *samples[0].VideoID != 42 {
		t.Errorf("video_id: want 42, got %v", samples[0].VideoID)
	}
}

// A reported audience lands in its own columns, beside the chatter count
// rather than on top of it — the two answer different questions, and the
// chatter series collected before viewers existed has to stay readable.
func TestRecordSample_RecordsAudience(t *testing.T) {
	db := setup(t)

	RecordSample(context.Background(), testConf, 5, Audience{Count: 137, Live: true, Reported: true}, 0)

	samples := allSamples(t, db)
	if len(samples) != 1 {
		t.Fatalf("expected 1 viewer_samples row, got %d", len(samples))
	}
	got := samples[0]
	if got.Count != 5 {
		t.Errorf("count = %d, want the chatter total 5", got.Count)
	}
	if got.Viewers == nil || *got.Viewers != 137 {
		t.Errorf("viewers = %v, want 137", got.Viewers)
	}
	if got.Live == nil || !*got.Live {
		t.Errorf("live = %v, want true", got.Live)
	}
}

// An unreported audience writes NULL, not 0. A platform that publishes no
// viewer count and one broadcasting to an empty room are different facts, and
// a rollup averaging them together reads the former as the latter.
func TestRecordSample_UnreportedAudienceWritesNull(t *testing.T) {
	db := setup(t)

	RecordSample(context.Background(), testConf, 5, Audience{}, 0)

	samples := allSamples(t, db)
	if len(samples) != 1 {
		t.Fatalf("expected 1 viewer_samples row, got %d", len(samples))
	}
	if samples[0].Viewers != nil || samples[0].Live != nil {
		t.Errorf("viewers = %v live = %v, want both NULL", samples[0].Viewers, samples[0].Live)
	}
}
