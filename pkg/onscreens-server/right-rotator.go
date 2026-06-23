package onscreensServer

import (
	"log/slog"
	"time"
)

// Matches the left rotator's 45s cadence. Beyond pacing the message swap, the
// interval doubles as a blank-recovery cadence: OBS renders this overlay via
// GPU-accelerated CEF offscreen rendering (BrowserHWAccel=true), whose
// shared-texture handoff occasionally gets a stale/blank frame stuck — and CEF
// only pushes a fresh frame when the rendered pixels actually change. A content
// rotation is what forces that repaint, so a shorter interval bounds how long a
// stuck-blank overlay stays blank. This was 90s, which left the right rotator
// visibly blank ~2x longer than the left (the asymmetry that surfaced the bug).
var rightRotatorUpdateFrequency = time.Duration(45 * time.Second)

// All right-rotator lines are platform-neutral (!location and !timewarp are both
// in the YouTube allowlist). Weight 2 reproduces the old duplicated entries.
var possibleRightMessages = []rotatorMessage{
	{Text: "Don't forget to follow :)", Weight: 2},
	{Text: "Try running `!location`", Weight: 2},
	{Text: "Try running `!timewarp`"},
	{Text: "Streaming 24 hours a day"},
}

// botlessRightMessages replace the command-hint right rotator on a bot-less
// YouTube instance (see botlessLeftMessages). This corner is the "subscribe
// here + journey flavor" half of the split: the own-stream call to action uses
// YouTube's "subscribe" (the left corner owns the Twitch CTA), so the two
// corners advertise different actions instead of both saying "follow on
// Twitch". On a bot-less stream these are mixed with the live date line (see
// botlessRightPool) — the info the !date command would return.
var botlessRightMessages = []rotatorMessage{
	{Text: "Driving across America, 24 hours a day"},
	{Text: "Subscribe to ride along"},
	{Text: "Slow-TV from the open road — just the drive"},
	{Text: "Real dashcam footage, streaming nonstop"},
}

// botlessRightPool is the bot-less right-rotator pool: the static promo lines
// plus the live date line ("📅 Monday January 2, 2006") when tripbot has pushed
// a fresh one. Paired with botlessLeftPool's location so the two corners show
// "where" and "when" rather than duplicating one field.
func botlessRightPool(now time.Time) []rotatorMessage {
	if _, date, ok := liveLocation.snapshot(now); ok && date != "" {
		return append([]rotatorMessage{{Text: "📅 " + date, Weight: liveDataWeight}}, botlessRightMessages...)
	}
	return botlessRightMessages
}

// newRightRotator constructs the right-rotator *Onscreen, primes it with
// a first message synchronously (so the OBS browser source has content
// to render the moment it polls — otherwise there's a brief race where
// the rotator is empty until the goroutine schedules), and kicks off the
// background loop that rotates the message every rightRotatorUpdateFrequency.
func newRightRotator() *Onscreen {
	slog.Info("creating onscreen", "kind", "right-rotator")
	osc := newOnscreen()
	osc.Show(rightRotatorContent())
	go rightRotatorLoop(osc)
	return osc
}

func rightRotatorLoop(osc *Onscreen) {
	for { // forever
		time.Sleep(time.Duration(rightRotatorUpdateFrequency))
		osc.Show(rightRotatorContent())
	}
}

// rightRotatorContent creates the content for the rightRotator
func rightRotatorContent() string {
	if botless() {
		return pickRotatorMessage(botlessRightPool(time.Now()))
	}
	// pick a weighted-random message for this platform
	return pickRotatorMessage(possibleRightMessages)
}
