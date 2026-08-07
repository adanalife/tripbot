package onscreensServer

import (
	"sync"
	"testing"
	"time"
)

// An Onscreen is written by the HTTP and NATS handlers while its own background
// loop sweeps it for expiry and the state endpoint reads it for the browser
// sources. Under -race this fails if any of those paths stops holding the lock.
func TestOnscreenConcurrentAccess(t *testing.T) {
	osc := newOnscreen(time.Millisecond)

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				osc.Show("shown")
				osc.ShowFor("timed", time.Millisecond)
				osc.SetContent("set")
				osc.SetState("restored", true)
				osc.Hide()
				_ = osc.Content()
				_ = osc.IsShowing()
				_ = osc.view()
			}
		}()
	}
	wg.Wait()
}

// ShowFor's window closes on its own: the background loop hides the overlay once
// the duration passes, which is what re-arms the browser source's rising edge.
func TestOnscreenShowForExpires(t *testing.T) {
	osc := newOnscreen(time.Millisecond)
	osc.ShowFor("timewarp", 20*time.Millisecond)
	if !osc.IsShowing() {
		t.Fatal("IsShowing = false immediately after ShowFor, want true")
	}

	deadline := time.Now().Add(2 * time.Second)
	for osc.IsShowing() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if osc.IsShowing() {
		t.Error("still showing well past the ShowFor duration, want hidden")
	}
	// Hide retains content — the persisted middle-text state depends on it.
	if got := osc.Content(); got != "timewarp" {
		t.Errorf("Content() after expiry = %q, want %q", got, "timewarp")
	}
}

// Show is the permanent state: no duration, so the sweeper must never hide it.
func TestOnscreenShowDoesNotExpire(t *testing.T) {
	osc := newOnscreen(time.Millisecond)
	osc.Show("permanent")
	time.Sleep(30 * time.Millisecond) // several sweeps at this interval
	if !osc.IsShowing() {
		t.Error("IsShowing = false after several sweeps, want a Show to persist")
	}
}
