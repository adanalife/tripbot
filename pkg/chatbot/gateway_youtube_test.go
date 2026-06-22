package chatbot

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/adanalife/tripbot/pkg/gateway"
)

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
