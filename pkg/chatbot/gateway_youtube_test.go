package chatbot

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/gateway"
)

func TestGatewayYouTubeSend_PostsToChat(t *testing.T) {
	var gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	// chatID is deliberately ignored — the gateway resolves the active chat.
	if err := (gatewayYouTubeSend{client: gateway.New(srv.URL)}).
		send(context.Background(), "ignored-chat-id", "hello"); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/v1/chat" {
		t.Errorf("path = %q, want /v1/chat", gotPath)
	}
	// identity is "" (gateway default) and the chatID is not sent.
	if !strings.Contains(gotBody, `"text":"hello"`) || !strings.Contains(gotBody, `"identity":""`) {
		t.Errorf("body = %q, want text=hello identity=empty", gotBody)
	}
	if strings.Contains(gotBody, "ignored-chat-id") {
		t.Errorf("body should not carry the chatID; got %q", gotBody)
	}
}

func TestGatewayYouTubeSend_ErrorsOnNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	if err := (gatewayYouTubeSend{client: gateway.New(srv.URL)}).
		send(context.Background(), "", "hi"); err == nil {
		t.Error("expected an error on a 502 from the gateway")
	}
}

func TestNewYouTubeSend_NoURLUsesInProcess(t *testing.T) {
	// With no YOUTUBE_API_URL there's no gateway to reach — fall back to the
	// in-process insert.
	prev := c.Conf.YouTubeAPIURL
	c.Conf.YouTubeAPIURL = ""
	defer func() { c.Conf.YouTubeAPIURL = prev }()

	if _, ok := newYouTubeSend().(realYouTubeSend); !ok {
		t.Error("expected realYouTubeSend when YOUTUBE_API_URL is empty")
	}
}

func TestNewYouTubeSend_WithURLUsesGateway(t *testing.T) {
	// A wired instance routes through the gateway unconditionally — no flag.
	prev := c.Conf.YouTubeAPIURL
	c.Conf.YouTubeAPIURL = "http://gateway-youtube:8080"
	defer func() { c.Conf.YouTubeAPIURL = prev }()

	if _, ok := newYouTubeSend().(gatewayYouTubeSend); !ok {
		t.Error("expected gatewayYouTubeSend when YOUTUBE_API_URL is set")
	}
}

func TestGatewayYouTubeChat_SayPostsToGateway(t *testing.T) {
	var gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	gatewayYouTubeChat{client: gateway.New(srv.URL)}.Say("/me hello")
	if gotPath != "/v1/chat" {
		t.Errorf("path = %q, want /v1/chat", gotPath)
	}
	// the Twitch-only "/me " prefix is stripped before sending to YouTube.
	if !strings.Contains(gotBody, `"text":"hello"`) {
		t.Errorf("body = %q, want text=hello (/me stripped)", gotBody)
	}
}

// inboundChatFunc adapts a func to the inboundChatClient seam.
type inboundChatFunc func(ctx context.Context, cursor string) (gateway.InboundChatPage, error)

func (f inboundChatFunc) InboundChat(ctx context.Context, cursor string) (gateway.InboundChatPage, error) {
	return f(ctx, cursor)
}

func TestGatewayYouTubeChatPoller_FeedsMessagesAndAdvancesCursor(t *testing.T) {
	pages := []gateway.InboundChatPage{
		{Messages: []gateway.InboundChatMessage{{Author: "A", Text: "!miles"}, {Author: "B", Text: "hi"}}, Cursor: "c1", Live: true, PollAfterMS: 1},
		{Cursor: "c2", Live: true, PollAfterMS: 1},
	}
	var gotCursors []string
	call := 0
	ctx, cancel := context.WithCancel(context.Background())
	fake := inboundChatFunc(func(_ context.Context, cursor string) (gateway.InboundChatPage, error) {
		gotCursors = append(gotCursors, cursor)
		if call >= len(pages) {
			cancel() // stop the loop after the scripted pages are drained
			return gateway.InboundChatPage{}, context.Canceled
		}
		p := pages[call]
		call++
		return p, nil
	})

	var handled []IncomingMessage
	p := &gatewayYouTubeChatPoller{
		client:    fake,
		handle:    func(_ context.Context, m IncomingMessage) { handled = append(handled, m) },
		pollFloor: time.Millisecond,
		errWait:   time.Millisecond,
	}
	p.Run(ctx)

	// Cursor starts empty, then forwards each page's cursor.
	if strings.Join(gotCursors, ",") != ",c1,c2" {
		t.Errorf("cursors = %v, want [\"\" c1 c2]", gotCursors)
	}
	if len(handled) != 2 || handled[0].User != "A" || handled[0].Text != "!miles" || handled[1].User != "B" {
		t.Errorf("handled = %+v, want A/!miles then B/hi", handled)
	}
}
