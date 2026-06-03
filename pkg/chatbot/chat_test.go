package chatbot

import (
	"encoding/json"
	"strings"
	"testing"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/eventbus"
)

// noopChat satisfies ChatClient for tests that don't care about chat output —
// it swallows everything. Tests that assert on output inject a recordingChat
// instead (see captureSay / the *_ViaIRC tests).
type noopChat struct{}

func (noopChat) Say(_ string)        {}
func (noopChat) Whisper(_, _ string) {}

// recordingChat captures every Say/Whisper call so tests can assert on
// chat output. All call records are appended in order.
type recordingChat struct {
	Says     []string          // ordered list of Say() messages
	Whispers []recordedWhisper // ordered list of Whisper() calls
}

type recordedWhisper struct {
	Username string
	Msg      string
}

func (r *recordingChat) Say(msg string) {
	r.Says = append(r.Says, msg)
}

func (r *recordingChat) Whisper(username, msg string) {
	r.Whispers = append(r.Whispers, recordedWhisper{Username: username, Msg: msg})
}

// Output returns all Say() messages joined by newline, mirroring the
// shape of captureSay()'s output() helper for easy migration.
func (r *recordingChat) Output() string {
	return strings.Join(r.Says, "\n")
}

// TestConsoleMirror_PublishesBotOutputToEventbus asserts the bot's own chat
// output is mirrored onto the event bus (so it shows in the admin live console
// — the platform doesn't echo our sent messages back). recordingNATS
// (nats_test.go) satisfies eventbus.Publisher. The inner ChatClient is a no-op
// so the test stays focused on the mirror.
func TestConsoleMirror_PublishesBotOutputToEventbus(t *testing.T) {
	rec := &recordingNATS{}
	saved := eventbus.Default
	eventbus.SetPublisher(rec)
	t.Cleanup(func() { eventbus.Default = saved })

	cm := consoleMirror{
		inner:       disconnectedChat{},
		env:         c.Conf.Environment,
		botUsername: c.Conf.BotUsername,
	}
	cm.Say("hello chat")

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
