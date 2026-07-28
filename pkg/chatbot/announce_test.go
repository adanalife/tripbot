package chatbot

import (
	"strings"
	"testing"

	"github.com/adanalife/tripbot/pkg/video"
)

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
