package eventsub

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	twitch "github.com/joeyak/go-twitch-eventsub/v3"
)

// A Helix POST that never answers must give up on its own: the library sends
// it with no client timeout, and Run can't return (so the caller can't redial)
// until the subscribe round does.
func TestSubscribe_GivesUpOnAHungHelix(t *testing.T) {
	release := make(chan struct{})
	helix := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	t.Cleanup(helix.Close)
	t.Cleanup(func() { close(release) })

	cfg := Config{ClientID: "cid", BroadcasterToken: "tok", BroadcasterUserID: "42",
		subscribeURL: helix.URL, subscribeTimeout: 50 * time.Millisecond}
	done := make(chan subscribeResult, 1)
	go func() {
		done <- subscribe(context.Background(), cfg, "sess", twitch.SubChannelFollow,
			map[string]string{"broadcaster_user_id": "42"})
	}()
	select {
	case got := <-done:
		if got != failed {
			t.Errorf("subscribe = %v, want failed", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("subscribe still waiting on a Helix that never answers")
	}
}
