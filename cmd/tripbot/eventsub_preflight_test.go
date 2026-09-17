package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/gateway"
	mytwitch "github.com/adanalife/tripbot/pkg/twitch"
)

// channelID is process-global, so a test that leaves one set decides the
// outcome of the next. Each case here starts from unresolved.
func withNoChannelID(t *testing.T) {
	t.Helper()
	mytwitch.SetChannelID("")
	t.Cleanup(func() { mytwitch.SetChannelID("") })
}

// The regression this file exists for. tripbot resolves its channel ID through
// the platform-gateway, and a co-restart of both — which is what a node crash
// produces — can put this call ahead of the gateway's readiness gate, so the
// Service still has no endpoints and the dial is refused. That gap closes in
// seconds, so it has to surface as a failed attempt the redial loop retries.
// Reporting it as a skip instead left EventSub dead for the life of the pod,
// with follows, subs, gifts, resubs and raids silently unannounced.
func TestEventSubPreflight_GatewayUnreachableIsRetryable(t *testing.T) {
	withNoChannelID(t)

	// A closed server refuses the connection the way an endpoint-less Service does.
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	srv.Close()

	tb := &Tripbot{
		cfg:     &c.TripbotConfig{ChannelName: "adanalife_"},
		gateway: gateway.New(srv.URL),
	}

	err := tb.eventSubPreflight(context.Background(), "broadcaster-token")
	if err == nil {
		t.Fatal("preflight returned nil on an unreachable gateway — an unready gateway must fail the attempt, not silently pass it through to a dial with no broadcaster id")
	}
	if errors.Is(err, errBroadcasterTokenUnloaded) {
		t.Errorf("err = %v, want a gateway error — a resolvable-later failure must not be charged the token-reload backoff", err)
	}
}

// Once the gateway answers, the id is resolved and cached for later attempts,
// so a recovered gateway needs exactly one round trip rather than one per dial.
func TestEventSubPreflight_ResolvesAndCachesChannelID(t *testing.T) {
	withNoChannelID(t)

	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		_, _ = w.Write([]byte(`{"id":"12345","login":"adanalife_"}`))
	}))
	defer srv.Close()

	tb := &Tripbot{
		cfg:     &c.TripbotConfig{ChannelName: "adanalife_"},
		gateway: gateway.New(srv.URL),
	}

	for i := range 2 {
		if err := tb.eventSubPreflight(context.Background(), "broadcaster-token"); err != nil {
			t.Fatalf("attempt %d: unexpected error: %v", i+1, err)
		}
	}
	if got := mytwitch.ChannelID(); got != "12345" {
		t.Errorf("ChannelID() = %q, want 12345 — eventsub dials with no broadcaster id otherwise", got)
	}
	if calls != 1 {
		t.Errorf("gateway calls = %d, want 1 — the resolved id should be reused across attempts", calls)
	}
}

// A missing broadcaster row is the one failure a redial cannot outpace, so it
// is reported distinctly: only the token-reload job can clear it, and the loop
// branches on this to wait for that job instead of spinning.
func TestEventSubPreflight_UnloadedTokenIsDistinct(t *testing.T) {
	withNoChannelID(t)

	tb := &Tripbot{cfg: &c.TripbotConfig{ChannelName: "adanalife_"}}

	err := tb.eventSubPreflight(context.Background(), "")
	if !errors.Is(err, errBroadcasterTokenUnloaded) {
		t.Fatalf("err = %v, want errBroadcasterTokenUnloaded — the loop branches on this to pace with the token reload", err)
	}
}

// With no gateway wired there is nothing to resolve the id with, so the attempt
// fails rather than dialing Twitch with an empty broadcaster id.
func TestEventSubPreflight_NoGatewayFails(t *testing.T) {
	withNoChannelID(t)

	tb := &Tripbot{cfg: &c.TripbotConfig{ChannelName: "adanalife_"}}

	if err := tb.eventSubPreflight(context.Background(), "broadcaster-token"); err == nil {
		t.Fatal("preflight returned nil with no gateway and no channel id")
	}
}
