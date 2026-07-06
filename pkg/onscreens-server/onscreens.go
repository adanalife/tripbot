package onscreensServer

import (
	"time"
)

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
}

// newOnscreen returns a freshly-initialized *Onscreen with the default
// sleep interval and an expiry pinned to "now" (i.e. ready to be shown
// for some duration). It also kicks off the background loop that hides
// expired onscreens.
func newOnscreen() *Onscreen {
	osc := &Onscreen{}
	osc.Content = ""
	osc.Expires = time.Now()
	osc.DontExpire = false
	osc.SleepInterval = time.Duration(defaultSleepInterval)
	// start the background loop
	go osc.backgroundLoop()
	return osc
}

// backgroundLoop will loop forever, hiding the Onscreen if needed
// TODO: add signal to end the loop
func (osc *Onscreen) backgroundLoop() {
	for { // forever
		if osc.IsShowing && osc.isExpired() {
			osc.Hide()
		}
		time.Sleep(osc.SleepInterval)
	}
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
