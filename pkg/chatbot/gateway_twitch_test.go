package chatbot

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/gateway"
)

func TestGatewayTwitch_FollowedAt_Following(t *testing.T) {
	followedAt := time.Now().Add(-72 * time.Hour).UTC().Truncate(time.Second)
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"followed_at":"` + followedAt.Format(time.RFC3339) + `"}`))
	}))
	defer srv.Close()

	when, ok := newGatewayTwitch(srv.URL).FollowedAt("Viewer1")
	if !ok {
		t.Fatal("expected ok=true for a follower")
	}
	if !when.Equal(followedAt) {
		t.Errorf("followed_at = %v, want %v", when, followedAt)
	}
	if gotPath != "/v1/followed-at/Viewer1" {
		t.Errorf("request path = %q, want /v1/followed-at/Viewer1", gotPath)
	}
}

func TestGatewayTwitch_FollowedAt_NotAFollower(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not a follower"}`))
	}))
	defer srv.Close()

	if _, ok := newGatewayTwitch(srv.URL).FollowedAt("viewer1"); ok {
		t.Error("expected ok=false on 404")
	}
}

func TestGatewayTwitch_FollowedAt_FailsClosedOnError(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
	}{
		{"upstream 502", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
		}},
		{"malformed body", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`not json`))
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(tt.handler)
			defer srv.Close()
			if _, ok := newGatewayTwitch(srv.URL).FollowedAt("viewer1"); ok {
				t.Error("expected ok=false (fail closed)")
			}
		})
	}
}

func TestGatewayTwitch_FollowedAt_TransportError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	srv.Close() // closed server → connection refused

	if _, ok := newGatewayTwitch(srv.URL).FollowedAt("viewer1"); ok {
		t.Error("expected ok=false on transport error")
	}
}

func TestNewTwitch_NoURLIsNoop(t *testing.T) {
	// A non-Twitch instance has no TWITCH_API_URL, so there's no gateway to
	// reach — newTwitch returns the fail-closed no-op adapter.
	if _, ok := newTwitch(&App{Cfg: testConf}).(noTwitch); !ok {
		t.Error("expected noTwitch when TWITCH_API_URL is empty")
	}
}

// Moving the Twitch chat connection into the gateway moved the only writer of
// the chat-connection gauge with it, and a live prod alert reads that gauge to
// catch "the pod is healthy but the bot is silent". The poll is the replacement
// signal, so what it records is pinned here.
//
// A failed poll must record 0: whatever the cause, the bot is not receiving
// chat, which is what the gauge means. Reporting nothing (the liveness hook's
// behaviour, where an unreachable gateway genuinely says nothing about whether
// the channel is streaming) would leave the gauge stuck at its last value and
// the alert blind.
func TestGatewayChatPollerReportsChatConnection(t *testing.T) {
	pages := []struct {
		page gateway.InboundChatPage
		err  error
	}{
		{page: gateway.InboundChatPage{Cursor: "c1", Live: true, PollAfterMS: 1}},
		{err: errors.New("gateway unreachable")},
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

	var reported []bool
	p := &gatewayChatPoller{
		client:           fake,
		handle:           func(context.Context, IncomingMessage) {},
		pollFloor:        time.Millisecond,
		errWait:          time.Millisecond,
		setChatConnected: func(c bool) { reported = append(reported, c) },
	}
	p.Run(ctx)

	if len(reported) < 3 || !reported[0] || reported[1] || reported[2] {
		t.Errorf("reported = %v, want connected, then false on the error, then false", reported)
	}
}

// The two chainable knobs Twitch needs. LongPolls has to zero the floor
// outright: the gateway signals "no need to wait" with a 0 interval, and any
// floor would override it and add latency to every message.
func TestTwitchPollerKnobs(t *testing.T) {
	base := (&App{Cfg: &c.TripbotConfig{}}).NewGatewayChatPoller("http://gw")
	if base.pollFloor == 0 {
		t.Fatal("default pollFloor = 0, want a floor for the platforms that re-query")
	}
	if base.setChatConnected != nil {
		t.Error("setChatConnected wired by default, want opt-in")
	}
	if p := base.LongPolls(); p.pollFloor != 0 {
		t.Errorf("LongPolls pollFloor = %v, want 0", p.pollFloor)
	}
	if p := base.ReportsChatConnection(); p.setChatConnected == nil {
		t.Error("ReportsChatConnection left setChatConnected nil")
	}
}
