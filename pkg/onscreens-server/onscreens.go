package onscreensServer

import (
	"time"
)

//TODO: these live in the background package and could/should
// be moved into this package
//TODO: we don't always need SleepInterval/Expires... some
// of these run forever (maybe refactor into ShowFor()?)

// defaultSleepInterval is how often onscreens refresh themselves
const defaultSleepInterval = time.Duration(5 * time.Second)

type Onscreen struct {
	Content       string
	Expires       time.Time
	DontExpire    bool
	SleepInterval time.Duration
	IsShowing     bool
}

// Snapshot is the JSON-serialisable view of an Onscreen used by the
// browser-source render endpoints in pkg/vlc-server.
type Snapshot struct {
	Content   string `json:"content"`
	IsShowing bool   `json:"showing"`
}

// Snapshot returns a point-in-time view of the Onscreen. Safe to call
// even when the Onscreen pointer is nil (returns the zero Snapshot).
func (osc *Onscreen) Snapshot() Snapshot {
	if osc == nil {
		return Snapshot{}
	}
	return Snapshot{
		Content:   osc.Content,
		IsShowing: osc.IsShowing,
	}
}

func New() *Onscreen {
	newOnscreen := &Onscreen{}
	newOnscreen.Content = ""
	newOnscreen.Expires = time.Now()
	newOnscreen.DontExpire = false
	newOnscreen.SleepInterval = time.Duration(defaultSleepInterval)
	// start the background loop
	go newOnscreen.backgroundLoop()
	return newOnscreen
}

// backgroundLoop will loop forever, hiding the Onscreen if needed
//TODO: add signal to end the loop
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
