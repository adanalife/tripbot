package chatbot

import (
	"context"
	"errors"
	"testing"
)

// recordingInsert captures InsertChatMessage calls for assertions.
type recordingInsert struct {
	calls []struct{ chatID, text string }
	err   error
}

func (r *recordingInsert) insert(_ context.Context, chatID, text string) error {
	r.calls = append(r.calls, struct{ chatID, text string }{chatID, text})
	return r.err
}

func TestYouTubeChat_SaySendsToBoundChat(t *testing.T) {
	rec := &recordingInsert{}
	binding := &liveChatBinding{}
	binding.Bind("chat-123")
	yc := youtubeChat{binding: binding, insert: rec.insert}

	yc.Say("hello youtube")

	if len(rec.calls) != 1 {
		t.Fatalf("insert called %d times, want 1", len(rec.calls))
	}
	if rec.calls[0].chatID != "chat-123" || rec.calls[0].text != "hello youtube" {
		t.Errorf("insert got (%q, %q)", rec.calls[0].chatID, rec.calls[0].text)
	}
}

func TestYouTubeChat_SayDropsWhenUnbound(t *testing.T) {
	rec := &recordingInsert{}
	yc := youtubeChat{binding: &liveChatBinding{}, insert: rec.insert}

	yc.Say("into the void")

	if len(rec.calls) != 0 {
		t.Fatalf("insert called %d times for an unbound chat, want 0", len(rec.calls))
	}
}

func TestYouTubeChat_SayStripsMeCommand(t *testing.T) {
	rec := &recordingInsert{}
	binding := &liveChatBinding{}
	binding.Bind("chat-123")
	yc := youtubeChat{binding: binding, insert: rec.insert}

	// the Chatter cron prefixes help messages with the Twitch IRC emote
	// command; YouTube would render it literally.
	yc.Say("/me try !help")

	if len(rec.calls) != 1 || rec.calls[0].text != "try !help" {
		t.Fatalf("want stripped text %q, got %+v", "try !help", rec.calls)
	}
}

func TestYouTubeChat_SaySurvivesInsertError(t *testing.T) {
	rec := &recordingInsert{err: errors.New("quota exceeded")}
	binding := &liveChatBinding{}
	binding.Bind("chat-123")
	yc := youtubeChat{binding: binding, insert: rec.insert}

	// must not panic; the error is logged and swallowed (ChatClient.Say
	// has no error return — same contract as twitchChat).
	yc.Say("doomed message")

	if len(rec.calls) != 1 {
		t.Fatalf("insert called %d times, want 1", len(rec.calls))
	}
}

func TestYouTubeChat_WhisperIsNoOp(t *testing.T) {
	rec := &recordingInsert{}
	binding := &liveChatBinding{}
	binding.Bind("chat-123")
	yc := youtubeChat{binding: binding, insert: rec.insert}

	yc.Whisper("someone", "psst")

	if len(rec.calls) != 0 {
		t.Fatalf("Whisper should not insert; called %d times", len(rec.calls))
	}
}

func TestLiveChatBinding_RebindSwitchesTarget(t *testing.T) {
	rec := &recordingInsert{}
	binding := &liveChatBinding{}
	binding.Bind("chat-old")
	yc := youtubeChat{binding: binding, insert: rec.insert}

	yc.Say("first")
	// broadcast ended; the poller re-discovers and re-binds (Phase B3 flow)
	binding.Bind("chat-new")
	yc.Say("second")

	if len(rec.calls) != 2 || rec.calls[0].chatID != "chat-old" || rec.calls[1].chatID != "chat-new" {
		t.Fatalf("rebind not honored: %+v", rec.calls)
	}
}
