package onscreensServer

import (
	"sync"
	"time"
)

//TODO: we don't always need SleepInterval/Expires... some
// of these run forever (maybe refactor into ShowFor()?)

// defaultSleepInterval is how often onscreens refresh themselves
const defaultSleepInterval = time.Duration(5 * time.Second)

// Onscreen is one browser-source overlay's live state. Every onscreen runs a
// background loop that hides it once its expiry passes, while the HTTP and NATS
// handlers set its content from their own goroutines — so the fields are
// unexported and reached only through the methods below, all of which hold mu.
//
// The JSON the OBS browser sources poll is onscreenView, produced by view()
// under the read lock; the encoder never walks this struct directly.
type Onscreen struct {
	mu            sync.RWMutex
	content       string
	isShowing     bool
	expires       time.Time
	dontExpire    bool
	sleepInterval time.Duration
}

// onscreenView is an Onscreen's JSON wire format, served by
// onscreensStateHandler. It's a snapshot: a plain value with no lock, so the
// handler can copy it and rewrite Content (the markdown-flagged overlays are
// rendered to HTML on the way out) without touching the live overlay.
type onscreenView struct {
	Content   string `json:"content"`
	IsShowing bool   `json:"showing"`
}

// newOnscreen returns a freshly-initialized *Onscreen with an expiry pinned to
// "now" (i.e. ready to be shown for some duration). It also kicks off the
// background loop that hides expired onscreens. sleepInterval paces that loop's
// expiry check — pass defaultSleepInterval unless the overlay needs to flip
// back to hidden faster than that.
func newOnscreen(sleepInterval time.Duration) *Onscreen {
	osc := &Onscreen{
		expires:       time.Now(),
		sleepInterval: sleepInterval,
	}
	// start the background loop
	go osc.backgroundLoop()
	return osc
}

// backgroundLoop will loop forever, hiding the Onscreen if needed
// TODO: add signal to end the loop
func (osc *Onscreen) backgroundLoop() {
	for { // forever
		osc.mu.Lock()
		if osc.isShowing && osc.expiredLocked() {
			osc.isShowing = false
		}
		interval := osc.sleepInterval
		osc.mu.Unlock()
		time.Sleep(interval)
	}
}

// expiredLocked reports whether the display window has passed. Caller holds mu.
func (osc *Onscreen) expiredLocked() bool {
	// return false if set to not expire
	if osc.dontExpire {
		return false
	}
	// return true if current date is after exp date
	return time.Now().After(osc.expires)
}

// extendLocked pushes the expiry out by dur. Caller holds mu.
func (osc *Onscreen) extendLocked(dur time.Duration) {
	// if it's expired, expire dur from now
	if osc.expiredLocked() {
		osc.expires = time.Now().Add(dur)
		return
	}
	// otherwise, add dur to the current expiry date
	osc.expires = osc.expires.Add(dur)
}

// Content returns the overlay's stored content — the raw source, before any
// markdown rendering the wire boundary applies.
func (osc *Onscreen) Content() string {
	osc.mu.RLock()
	defer osc.mu.RUnlock()
	return osc.content
}

// IsShowing reports whether the overlay is currently visible.
func (osc *Onscreen) IsShowing() bool {
	osc.mu.RLock()
	defer osc.mu.RUnlock()
	return osc.isShowing
}

// view snapshots the fields the browser sources render.
func (osc *Onscreen) view() onscreenView {
	osc.mu.RLock()
	defer osc.mu.RUnlock()
	return onscreenView{Content: osc.content, IsShowing: osc.isShowing}
}

// Show makes an onscreen visible until hidden
func (osc *Onscreen) Show(content string) {
	osc.mu.Lock()
	defer osc.mu.Unlock()
	osc.dontExpire = true
	osc.isShowing = true
	osc.content = content
}

// ShowFor makes an Onscreen visible for a duration of time
func (osc *Onscreen) ShowFor(content string, dur time.Duration) {
	osc.mu.Lock()
	defer osc.mu.Unlock()
	osc.dontExpire = false
	osc.extendLocked(dur)
	osc.isShowing = true
	osc.content = content
}

// Hide will remove an onscreen from the screen
func (osc *Onscreen) Hide() {
	osc.mu.Lock()
	defer osc.mu.Unlock()
	osc.isShowing = false
}

func (osc *Onscreen) SetContent(content string) {
	osc.mu.Lock()
	defer osc.mu.Unlock()
	osc.content = content
}

// SetState sets content and visibility together, for restoring a persisted
// overlay: the pair lands as one update, so neither the background loop nor a
// polling browser source can observe the new text while it still reads hidden.
func (osc *Onscreen) SetState(content string, showing bool) {
	osc.mu.Lock()
	defer osc.mu.Unlock()
	osc.content = content
	osc.isShowing = showing
}
