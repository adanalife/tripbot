package chatbot

import (
	"strings"
	"testing"

	"github.com/adanalife/tripbot/pkg/video"
)

// A new follower lands one follow event naming them — the durable rows a
// "followers gained" recap counts, and the only place the EventSub notice
// survives past the chat shout.
func TestAnnounceNewFollower_RecordsFollowEvent(t *testing.T) {
	app := newTestApp(video.Video{})
	rec := &recordingEvents{}
	app.Events = rec

	app.AnnounceNewFollower("viewer1")

	if len(rec.Follows) != 1 {
		t.Fatalf("follows = %d, want 1", len(rec.Follows))
	}
	if rec.Follows[0] != "viewer1" {
		t.Errorf("follow username = %q, want viewer1", rec.Follows[0])
	}
}

// TestAnnounceNewFollower_NudgesCommands asserts the follow thank-you points
// new followers at the discovery command — a follow is the first moment a
// viewer learns the bot is interactive. The eventbus emit rides along
// unpublished (no NATS conn in tests), so this stays focused on the chat copy.
func TestAnnounceNewFollower_NudgesCommands(t *testing.T) {
	app := newTestApp(video.Video{})
	rec := &recordingChat{}
	app.Chat = rec

	app.AnnounceNewFollower("viewer1")

	if len(rec.Says) != 1 {
		t.Fatalf("expected exactly one Say() call, got %d: %v", len(rec.Says), rec.Says)
	}
	if !strings.Contains(rec.Says[0], "@viewer1") {
		t.Errorf("expected @username in follow thank-you, got %q", rec.Says[0])
	}
	if !strings.Contains(rec.Says[0], "!commands") {
		t.Errorf("expected a !commands nudge in follow thank-you, got %q", rec.Says[0])
	}
}

// --- RecordRaid ---

// An incoming raid records one raid event carrying the raiding channel and
// party size, stamped with the airing clip and playhead — the row a per-clip
// audience rollup uses to discount the spike. Nothing is said in chat: Twitch
// announces the raid natively.
func TestRecordRaid_RecordsRaidEventWithAiring(t *testing.T) {
	app := newTestApp(video.Video{ID: 77})
	rec := &recordingEvents{}
	app.Events = rec
	chat := &recordingChat{}
	app.Chat = chat

	app.RecordRaid("somechannel", 25)

	if len(rec.Raids) != 1 {
		t.Fatalf("raids = %d, want 1", len(rec.Raids))
	}
	got := rec.Raids[0]
	if got.From != "somechannel" || got.Viewers != 25 {
		t.Errorf("raid = %+v, want somechannel raiding with 25 viewers", got)
	}
	if got.VideoID != 77 {
		t.Errorf("video id = %d, want 77", got.VideoID)
	}
	if got.TsSec == nil {
		t.Error("ts_sec = nil, want the playhead stamped")
	}
	if len(chat.Says) != 0 {
		t.Errorf("chat says = %v, want none for a raid", chat.Says)
	}
}

// A raid arriving before the player has a current clip still records, with
// empty airing context — the raid happened whether or not the playhead is
// known.
func TestRecordRaid_NoPlayerStillRecords(t *testing.T) {
	app := newTestApp(video.Video{})
	rec := &recordingEvents{}
	app.Events = rec
	app.Video = nil

	app.RecordRaid("somechannel", 25)

	if len(rec.Raids) != 1 {
		t.Fatalf("raids = %d, want 1", len(rec.Raids))
	}
	got := rec.Raids[0]
	if got.VideoID != 0 {
		t.Errorf("video id = %d, want 0 (writes NULL)", got.VideoID)
	}
	if got.TsSec != nil {
		t.Errorf("ts_sec = %v, want nil (writes NULL)", *got.TsSec)
	}
}
