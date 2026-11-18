package onscreensServer

import (
	"testing"
	"time"

	"github.com/nats-io/nats.go"
)

func TestHandleSpeedUpdate(t *testing.T) {
	s := &Server{Speedometer: newSpeedometer()}
	defer s.Speedometer.stop()

	s.handleSpeedUpdate(&nats.Msg{Data: []byte(`{"emitted_at":"2026-10-10T00:00:00Z","mph":57.6}`)})
	if !s.Speedometer.IsShowing() || s.Speedometer.Content() != "58" {
		t.Fatalf("after a reading: showing=%v content=%q, want true \"58\"", s.Speedometer.IsShowing(), s.Speedometer.Content())
	}
	// No mph is "nothing honest to show", not a stale feed: hide now.
	s.handleSpeedUpdate(emptyMsg("tripbot.test.onscreens.speed.update.twitch"))
	if s.Speedometer.IsShowing() {
		t.Error("after an update with no mph: still showing, want hidden")
	}
}

// A feed re-arms the overlay every tick; each one must reset the window, not
// add to it, or a speedometer whose feed stops stays up for as long as it ran.
func TestRefreshResetsTheExpiry(t *testing.T) {
	osc := newOnscreen(time.Hour)
	defer osc.stop()
	for range 5 {
		osc.Refresh("1", time.Minute)
	}
	osc.mu.RLock()
	left := time.Until(osc.expires)
	osc.mu.RUnlock()
	if left > time.Minute {
		t.Errorf("expiry %v away after five refreshes of a minute, want ≤ 1m", left.Round(time.Second))
	}
}
