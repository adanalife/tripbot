package onscreensServer

import (
	"sync"
	"testing"
	"testing/synctest"
	"time"
)

// sweepInterval paces the expiry loop under test. synctest puts the bubble on a
// fake clock, so the value is arbitrary and costs no wall time: every sweep
// lands on an exact interval boundary instead of racing the real scheduler.
const sweepInterval = time.Second

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
	synctest.Test(t, func(t *testing.T) {
		osc := newOnscreen(sweepInterval)
		defer osc.stop() // lets the loop exit before the bubble closes
		osc.ShowFor("timewarp", 3*sweepInterval)
		synctest.Wait()
		if !osc.IsShowing() {
			t.Fatal("IsShowing = false immediately after ShowFor, want true")
		}

		// The first sweep past the window is the one that hides it.
		time.Sleep(4 * sweepInterval)
		synctest.Wait()
		if osc.IsShowing() {
			t.Error("still showing past the ShowFor duration, want hidden")
		}
		// Hide retains content — the persisted middle-text state depends on it.
		if got := osc.Content(); got != "timewarp" {
			t.Errorf("Content() after expiry = %q, want %q", got, "timewarp")
		}
	})
}

// Show is the permanent state: no duration, so the sweeper must never hide it.
func TestOnscreenShowDoesNotExpire(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		osc := newOnscreen(sweepInterval)
		defer osc.stop()
		osc.Show("permanent")
		time.Sleep(30 * sweepInterval) // several sweeps at this interval
		synctest.Wait()
		if !osc.IsShowing() {
			t.Error("IsShowing = false after several sweeps, want a Show to persist")
		}
	})
}
