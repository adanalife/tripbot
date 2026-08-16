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
