package chatbot

import (
	"encoding/json"
	"testing"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/eventbus"
)

// TestSay_PublishesBotOutputToEventbus asserts the bot's own chat output is
// mirrored onto the event bus (so it shows in the admin live console — Twitch
// doesn't echo our sent messages back via PrivateMessage). recordingNATS
// (nats_test.go) satisfies eventbus.Publisher too, so we reuse it.
func TestSay_PublishesBotOutputToEventbus(t *testing.T) {
	rec := &recordingNATS{}
	saved := eventbus.Default
	eventbus.SetPublisher(rec)
	t.Cleanup(func() { eventbus.Default = saved })

	// client is nil in tests, so client.Say panics — but the eventbus emit
	// happens first. Catch the panic and assert the publish landed.
	func() {
		defer func() { _ = recover() }()
		Say("hello chat")
	}()

	if len(rec.Publishes) != 1 {
		t.Fatalf("expected 1 publish, got %d", len(rec.Publishes))
	}
	p := rec.Publishes[0]
	if want := "tripbot." + c.Conf.Environment + ".chat.message"; p.Subject != want {
		t.Errorf("subject = %q, want %q", p.Subject, want)
	}
	var ev eventbus.ChatMessage
	if err := json.Unmarshal(p.Payload, &ev); err != nil {
		t.Fatalf("bad payload: %v", err)
	}
	if ev.Username != c.Conf.BotUsername {
		t.Errorf("username = %q, want bot %q", ev.Username, c.Conf.BotUsername)
	}
	if ev.Text != "hello chat" {
		t.Errorf("text = %q, want hello chat", ev.Text)
	}
}
