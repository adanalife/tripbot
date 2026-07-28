package chatbot

import (
	"context"
	"errors"
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

func TestGatewayChatPoller_FeedsMessagesAndAdvancesCursor(t *testing.T) {
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
	p := &gatewayChatPoller{
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

// The poller routes by kind: comments to the command path, gifts to the effect
// path, and an unrecognized kind to neither — a newer gateway can emit a kind
// this build has never seen, and its empty Text would otherwise reach the
// command parser as a blank line.
func TestGatewayChatPollerRoutesByKind(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	page := gateway.InboundChatPage{
		Messages: []gateway.InboundChatMessage{
			{Author: "A", AuthorID: "1", Text: "!timewarp"},
			{Author: "B", AuthorID: "2", Kind: gateway.KindGift,
				Gift: &gateway.Gift{ID: "5655", Name: "Rose", Count: 3, Diamonds: 1}},
			{Author: "C", AuthorID: "3", Kind: gateway.KindGift}, // malformed: no payload
			{Author: "D", AuthorID: "4", Kind: "like"},           // unknown kind
		},
		Cursor: "c1", Live: true, PollAfterMS: 1,
	}

	sent := false
	fake := inboundChatFunc(func(_ context.Context, _ string) (gateway.InboundChatPage, error) {
		if sent {
			cancel()
			return gateway.InboundChatPage{}, context.Canceled
		}
		sent = true
		return page, nil
	})

	var msgs []IncomingMessage
	var gifts []IncomingGift
	p := &gatewayChatPoller{
		client:     fake,
		handle:     func(_ context.Context, m IncomingMessage) { msgs = append(msgs, m) },
		handleGift: func(_ context.Context, g IncomingGift) { gifts = append(gifts, g) },
		pollFloor:  time.Millisecond,
		errWait:    time.Millisecond,
	}
	p.Run(ctx)

	if len(msgs) != 1 || msgs[0].User != "A" || msgs[0].Text != "!timewarp" {
		t.Errorf("chat handled = %+v, want only A/!timewarp", msgs)
	}
	if len(gifts) != 1 {
		t.Fatalf("gifts handled = %+v, want only the one with a payload", gifts)
	}
	want := IncomingGift{User: "B", UserID: "2", Name: "Rose", Count: 3, Value: 3}
	if gifts[0] != want {
		t.Errorf("gift = %+v, want %+v", gifts[0], want)
	}
}

// Liveness reporting tracks each page's Live flag, and a poll that fails must
// leave it untouched: an unreachable gateway says nothing about whether the
// channel is live, and recording offline there would read as a silent
// disconnect and page.
func TestGatewayChatPollerReportsLiveness(t *testing.T) {
	pages := []struct {
		page gateway.InboundChatPage
		err  error
	}{
		{page: gateway.InboundChatPage{Cursor: "c1", Live: true, PollAfterMS: 1}},
		{err: errors.New("connection refused")}, // must not record
		{page: gateway.InboundChatPage{Cursor: "c2", Live: false, PollAfterMS: 1}},
	}

	call := 0
	ctx, cancel := context.WithCancel(context.Background())
	fake := inboundChatFunc(func(_ context.Context, _ string) (gateway.InboundChatPage, error) {
		if call >= len(pages) {
			cancel()
			return gateway.InboundChatPage{}, context.Canceled
		}
		p := pages[call]
		call++
		return p.page, p.err
	})

	var recorded []bool
	p := &gatewayChatPoller{
		client:    fake,
		handle:    func(context.Context, IncomingMessage) {},
		pollFloor: time.Millisecond,
		errWait:   time.Millisecond,
		setLive:   func(live bool) { recorded = append(recorded, live) },
	}
	p.Run(ctx)

	if len(recorded) != 2 || !recorded[0] || recorded[1] {
		t.Errorf("recorded = %v, want [true false] (the failed poll records nothing)", recorded)
	}
}

// Liveness is opt-in: the default poller reports none, so a platform whose
// liveness comes from a broadcast-discovery tick doesn't get a second writer
// fighting it on the same gauge.
func TestGatewayChatPollerLivenessIsOptIn(t *testing.T) {
	if (&gatewayChatPoller{}).setLive != nil {
		t.Error("setLive is set by default, want nil until ReportsLiveness")
	}
	if (&App{}).NewGatewayChatPoller("http://gateway.invalid").setLive != nil {
		t.Error("NewGatewayChatPoller reports liveness, want opt-in via ReportsLiveness")
	}
	if (&App{}).NewGatewayChatPoller("http://gateway.invalid").ReportsLiveness().setLive == nil {
		t.Error("ReportsLiveness left setLive nil")
	}
}
