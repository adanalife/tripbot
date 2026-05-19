package onscreensServer

import (
	"sync"
	"time"
)

//TODO: these live in the background package and could/should
// be moved into this package
//TODO: we don't always need SleepInterval/Expires... some
// of these run forever (maybe refactor into ShowFor()?)

// defaultSleepInterval is how often onscreens refresh themselves
const defaultSleepInterval = time.Duration(5 * time.Second)

// Onscreen is also the JSON wire format served by the browser-source
// state endpoint in pkg/vlc-server, so Content/IsShowing carry json
// tags and the bookkeeping fields are skipped.
type Onscreen struct {
	Content       string        `json:"content"`
	IsShowing     bool          `json:"showing"`
	Expires       time.Time     `json:"-"`
	DontExpire    bool          `json:"-"`
	SleepInterval time.Duration `json:"-"`

	// stop is closed by shutdown() to signal background goroutines
	// (the expiry sweeper plus the optional rotator loop) to exit. wg
	// is incremented for each goroutine spawned against this onscreen
	// so shutdown() can block until they've all returned. Both are
	// zero-value-usable; newOnscreen makes them explicit.
	stop chan struct{}
	wg   sync.WaitGroup

	// stopOnce guards shutdown() so multiple closes don't panic — the
	// Server calls shutdown on every onscreen on every Shutdown() call,
	// and tests sometimes Shutdown twice for cleanup.
	stopOnce sync.Once
}

// newOnscreen returns a freshly-initialized *Onscreen with the default
// sleep interval and an expiry pinned to "now" (i.e. ready to be shown
// for some duration). It also kicks off the background loop that hides
// expired onscreens; that loop exits when shutdown() is called.
func newOnscreen() *Onscreen {
	osc := &Onscreen{
		Content:       "",
		Expires:       time.Now(),
		DontExpire:    false,
		SleepInterval: time.Duration(defaultSleepInterval),
		stop:          make(chan struct{}),
	}
	// start the background loop
	osc.wg.Add(1)
	go osc.backgroundLoop()
	return osc
}

// backgroundLoop hides expired onscreens on a fixed cadence. It exits
// when osc.stop is closed by shutdown(). Sleep is implemented via a
// select-on-timer so the goroutine can react to stop without waiting
// out a full SleepInterval.
func (osc *Onscreen) backgroundLoop() {
	defer osc.wg.Done()
	for {
		if osc.IsShowing && osc.isExpired() {
			osc.Hide()
		}
		t := time.NewTimer(osc.SleepInterval)
		select {
		case <-osc.stop:
			t.Stop()
			return
		case <-t.C:
		}
	}
}

// signalStop closes osc.stop so background goroutines see the signal.
// Idempotent — guarded by stopOnce so repeated calls don't panic.
func (osc *Onscreen) signalStop() {
	osc.stopOnce.Do(func() {
		close(osc.stop)
	})
}

func (osc *Onscreen) isExpired() bool {
	// return false if set to not expire
	if osc.DontExpire {
		return false
	}
	// return true if current date is after exp date
	return time.Now().After(osc.Expires)
}

func (osc *Onscreen) Extend(dur time.Duration) {
	// if it's expired, expire dur from now
	if osc.isExpired() {
		osc.Expires = time.Now().Add(dur)
		return
	}
	// otherwise, add dur to the current expiry date
	osc.Expires = osc.Expires.Add(dur)
}

// Show makes an onscreen visible until hidden
func (osc *Onscreen) Show(content string) {
	osc.DontExpire = true
	osc.IsShowing = true
	osc.Content = content
}

// ShowFor makes an Onscreen visible for a duration of time
func (osc *Onscreen) ShowFor(content string, dur time.Duration) {
	osc.DontExpire = false
	osc.Extend(dur)
	osc.IsShowing = true
	osc.Content = content
}

// Hide will remove an onscreen from the screen
func (osc *Onscreen) Hide() {
	osc.IsShowing = false
}

func (osc *Onscreen) SetContent(content string) {
	osc.Content = content
}
