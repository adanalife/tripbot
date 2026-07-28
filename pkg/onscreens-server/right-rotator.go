package onscreensServer

import (
	"time"

	c "github.com/adanalife/tripbot/pkg/config/onscreens-server"
	rot "github.com/adanalife/tripbot/pkg/rotator"
)

// Matches the left rotator's 45s cadence. Beyond pacing the message swap, the
// interval doubles as a blank-recovery cadence: OBS renders this overlay via
// GPU-accelerated CEF offscreen rendering (BrowserHWAccel=true), whose
// shared-texture handoff occasionally gets a stale/blank frame stuck — and CEF
// only pushes a fresh frame when the rendered pixels actually change. A content
// rotation is what forces that repaint, so a shorter interval bounds how long a
// stuck-blank overlay stays blank.
var rightRotatorUpdateFrequency = time.Duration(45 * time.Second)

// rightLiveLine is the promoMode right-rotator live-data line: the current date
// ("📅 Monday January 2, 2006") when tripbot has pushed a fresh one. Paired with
// leftLiveLine's location so the two corners show "when" and "where" rather than
// duplicating one field.
//
// Generated from the playing clip rather than authored, so it's deliberately not
// part of the console-editable copy — it's mixed into the promo pool here at
// render time.
func rightLiveLine(now time.Time) (rot.Message, bool) {
	if _, date, ok := liveLocation.snapshot(now); ok && date != "" {
		return rot.Message{Text: "📅 " + date, Weight: liveDataWeight}, true
	}
	return rot.Message{}, false
}

// newRightRotator configures the right corner, seeded with cfgCopy — the copy
// compiled into the binary at startup, console-edited copy once
// RestoreRotatorCopy has run. The caller pairs it with the left rotator and
// calls start().
//
// This is the tighter corner of the two: its grey-box underlay is 369px against
// the left's 564px (rot.BudgetFor), so its lines have to be shorter to avoid
// shrinking to the font floor and wrapping to a second line.
func newRightRotator(cfg *c.OnscreensServerConfig, cfgCopy rot.Config) *rotator {
	r := &rotator{
		cfg:      cfg,
		kind:     "right-rotator",
		freq:     rightRotatorUpdateFrequency,
		liveLine: rightLiveLine,
	}
	// No rare message on this corner — the easter egg is the left corner's.
	r.setCopy(cfgCopy.Right, "")
	return r
}
