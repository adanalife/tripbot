package chatbot

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/adanalife/tripbot/pkg/video"
	myyoutube "github.com/adanalife/tripbot/pkg/youtube"
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

func TestHandleYouTubeMessage_RunsCommandWithoutLogin(t *testing.T) {
	app := newTestApp(video.Video{})
	rec := &recordingChat{}
	app.Chat = rec
	// UserSessions stays nil: if HandleYouTubeMessage attempted the
	// LoginIfNecessary step the Twitch path does, this test would panic —
	// passing proves the skip-login contract.

	app.HandleYouTubeMessage(context.Background(), IncomingMessage{User: "YouTubeViewer", Text: "!help"})

	if len(rec.Says) == 0 {
		t.Fatal("expected !help to dispatch and reply via App.Chat")
	}
}

func TestHandleYouTubeMessage_NonCommandIsQuiet(t *testing.T) {
	app := newTestApp(video.Video{})
	rec := &recordingChat{}
	app.Chat = rec

	app.HandleYouTubeMessage(context.Background(), IncomingMessage{User: "YouTubeViewer", Text: "nice stream"})

	if len(rec.Says) != 0 {
		t.Fatalf("plain chatter should not reply; got %v", rec.Says)
	}
}

// pollerScript drives youtubeChatPoller.Run through a scripted sequence of
// list responses, canceling the context when the script is exhausted.
type pollerScript struct {
	mu            sync.Mutex
	pages         []func() (*myyoutube.LiveChatPage, error)
	calls         []string // pageTokens seen, for cursor assertions
	cancel        context.CancelFunc
	discoverRet   func() (string, error)
	discoverCalls int
}

func (s *pollerScript) discover(_ context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.discoverCalls++
	return s.discoverRet()
}

func (s *pollerScript) list(_ context.Context, _, pageToken string) (*myyoutube.LiveChatPage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, pageToken)
	if len(s.pages) == 0 {
		s.cancel()
		return nil, context.Canceled
	}
	next := s.pages[0]
	s.pages = s.pages[1:]
	return next()
}

func newScriptedPoller(t *testing.T, app *App, binding *liveChatBinding, s *pollerScript) (*youtubeChatPoller, context.Context) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	s.cancel = cancel
	return &youtubeChatPoller{
		app:            app,
		binding:        binding,
		discover:       s.discover,
		list:           s.list,
		ownChannelID:   func() string { return "UCbot" },
		pollFloor:      time.Millisecond,
		rediscoverWait: time.Millisecond,
		quotaWait:      time.Millisecond,
	}, ctx
}

func page(token string, msgs ...myyoutube.LiveChatMessage) func() (*myyoutube.LiveChatPage, error) {
	return func() (*myyoutube.LiveChatPage, error) {
		return &myyoutube.LiveChatPage{Messages: msgs, NextPageToken: token, PollAfter: time.Millisecond}, nil
	}
}

func TestPoller_SkipsBacklogThenProcesses(t *testing.T) {
	app := newTestApp(video.Video{})
	rec := &recordingChat{}
	app.Chat = rec

	binding := &liveChatBinding{}
	binding.Bind("chat-1") // pre-bound, as after ConnectYouTube
	script := &pollerScript{pages: []func() (*myyoutube.LiveChatPage, error){
		// page 1 is backlog: this !help must NOT dispatch
		page("t1", myyoutube.LiveChatMessage{AuthorChannelID: "UCviewer", Author: "Old", Text: "!help"}),
		// page 2 is live traffic: this one must dispatch
		page("t2", myyoutube.LiveChatMessage{AuthorChannelID: "UCviewer", Author: "Now", Text: "!help"}),
	}}
	p, ctx := newScriptedPoller(t, app, binding, script)

	p.Run(ctx)

	if len(rec.Says) != 1 {
		t.Fatalf("want exactly 1 dispatched reply (backlog skipped), got %d: %v", len(rec.Says), rec.Says)
	}
	// the cursor must advance even across the discarded backlog page
	if len(script.calls) < 2 || script.calls[0] != "" || script.calls[1] != "t1" {
		t.Errorf("pageToken sequence wrong: %v", script.calls)
	}
}

func TestPoller_SkipsOwnEchoedMessages(t *testing.T) {
	app := newTestApp(video.Video{})
	rec := &recordingChat{}
	app.Chat = rec

	binding := &liveChatBinding{}
	binding.Bind("chat-1")
	script := &pollerScript{pages: []func() (*myyoutube.LiveChatPage, error){
		page("t1"), // backlog page (empty)
		page("t2",
			myyoutube.LiveChatMessage{AuthorChannelID: "UCbot", Author: "TheChannel", Text: "!help"},
			myyoutube.LiveChatMessage{AuthorChannelID: "UCviewer", Author: "Viewer", Text: "!help"},
		),
	}}
	p, ctx := newScriptedPoller(t, app, binding, script)

	p.Run(ctx)

	if len(rec.Says) != 1 {
		t.Fatalf("own echoed message should be filtered; got %d replies: %v", len(rec.Says), rec.Says)
	}
}

func TestPoller_RebindsAfterChatGone(t *testing.T) {
	app := newTestApp(video.Video{})
	app.Chat = &recordingChat{}

	binding := &liveChatBinding{}
	binding.Bind("chat-old")
	script := &pollerScript{pages: []func() (*myyoutube.LiveChatPage, error){
		func() (*myyoutube.LiveChatPage, error) { return nil, myyoutube.ErrChatGone },
	}}
	script.discoverRet = func() (string, error) { return "chat-new", nil }
	p, ctx := newScriptedPoller(t, app, binding, script)
	// after ErrChatGone the poller rediscovers, binds chat-new, and calls
	// list again; the script is exhausted by then, so that call cancels the
	// ctx and Run exits.

	p.Run(ctx)

	if got := binding.ID(); got != "chat-new" {
		t.Errorf("binding after ErrChatGone = %q, want chat-new (rediscovered)", got)
	}
	if script.discoverCalls == 0 {
		t.Error("discover never called after ErrChatGone")
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
