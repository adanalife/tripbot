package onscreensServer

import (
	"time"

	c "github.com/adanalife/tripbot/pkg/config/onscreens-server"
	rot "github.com/adanalife/tripbot/pkg/rotator"
)

var leftRotatorUpdateFrequency = time.Duration(45 * time.Second)

// liveDataWeight biases the live location/date line over the static promo lines
// in the promo pools — the data is the headline (it's what the !location /
// !date commands would return), the promo is the remainder. Tunable; ~50-65%
// data against the default promo weights.
const liveDataWeight = 6

// leftLiveLine is the promoMode left-rotator live-data line: the current location
// ("📍 City, State") when tripbot has pushed a fresh one. Paired with
// rightLiveLine's date so the two corners show "where" and "when" rather than
// duplicating one field.
//
// Generated from the playing clip rather than authored, so it's deliberately not
// part of the console-editable copy — it's mixed into the promo pool here at
// render time.
func leftLiveLine(now time.Time) (rot.Message, bool) {
	if loc, _, ok := liveLocation.snapshot(now); ok && loc != "" {
		return rot.Message{Text: "📍 " + loc, Weight: liveDataWeight}, true
	}
	return rot.Message{}, false
}

// newLeftRotator configures the left corner, seeded with cfgCopy — the copy
// compiled into the binary at startup, console-edited copy once
// RestoreRotatorCopy has run. The caller pairs it with the right rotator and
// calls start().
func newLeftRotator(cfg *c.OnscreensServerConfig, cfgCopy rot.Config) *rotator {
	r := &rotator{
		cfg:      cfg,
		kind:     "left-rotator",
		freq:     leftRotatorUpdateFrequency,
		liveLine: leftLiveLine,
	}
	// The easter egg is this corner's alone.
	r.setCopy(cfgCopy.Left, cfgCopy.RareMessage)
	return r
}
