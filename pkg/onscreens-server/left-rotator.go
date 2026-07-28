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

// leftLiveLine is the promoMode left-rotator live-data line: where the clip was
// filmed. Paired with rightLiveLine's date so the two corners show "where" and
// "when" rather than duplicating one field. Renders only while tripbot is pushing
// a location — an unresolved $variable makes the line ineligible.
//
// Not part of the console-editable copy: it's the one line each promo corner is
// guaranteed to be able to show, so it ships with the binary rather than being
// something an edit could leave a platform without.
var leftLiveLine = rot.Message{Text: "📍 $location", Weight: liveDataWeight}

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
