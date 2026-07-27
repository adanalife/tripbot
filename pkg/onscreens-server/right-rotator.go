package onscreensServer

import (
	c "github.com/adanalife/tripbot/pkg/config/onscreens-server"
	"time"
)

// Matches the left rotator's 45s cadence. Beyond pacing the message swap, the
// interval doubles as a blank-recovery cadence: OBS renders this overlay via
// GPU-accelerated CEF offscreen rendering (BrowserHWAccel=true), whose
// shared-texture handoff occasionally gets a stale/blank frame stuck — and CEF
// only pushes a fresh frame when the rendered pixels actually change. A content
// rotation is what forces that repaint, so a shorter interval bounds how long a
// stuck-blank overlay stays blank.
var rightRotatorUpdateFrequency = time.Duration(45 * time.Second)

// All right-rotator lines are platform-neutral (!location and !timewarp are both
// in the YouTube allowlist). Weight 2 makes the follow and !location hints
// twice as likely as the unweighted lines.
var possibleRightMessages = []rotatorMessage{
	{Text: "Don't forget to follow :)", Weight: 2},
	{Text: "Try running `!location`", Weight: 2},
	{Text: "Try running `!timewarp`"},
	{Text: "Streaming 24 hours a day"},
}

// promoRightMessages replace the full command-hint right rotator in promoMode
// (see promoLeftMessages). This corner owns "here is what you're watching" —
// the journey flavor plus each platform's own-platform call to action, worded
// in that platform's verb (YouTube subscribes, TikTok follows) so the two
// corners never advertise the same action at once.
//
// On a promoMode stream these are mixed with the live date line (see
// rightLiveLine) — the info the !date command would return.
var promoRightMessages = []rotatorMessage{
	{Text: "Driving across America, 24 hours a day"},
	{Text: "Subscribe to ride along", Platforms: []string{platformYouTube}},
	{Text: "Follow to ride along", Platforms: []string{platformTikTok, platformInstagram}},
	{Text: "Slow-TV from the open road — just the drive"},
	{Text: "Real dashcam footage, streaming nonstop"},
}

// rightLiveLine is the promoMode right-rotator live-data line: the current date
// ("📅 Monday January 2, 2006") when tripbot has pushed a fresh one. Paired with
// leftLiveLine's location so the two corners show "when" and "where" rather than
// duplicating one field.
func rightLiveLine(now time.Time) (rotatorMessage, bool) {
	if _, date, ok := liveLocation.snapshot(now); ok && date != "" {
		return rotatorMessage{Text: "📅 " + date, Weight: liveDataWeight}, true
	}
	return rotatorMessage{}, false
}

// newRightRotator configures the right corner. The caller pairs it with the left
// rotator and calls start().
func newRightRotator(cfg *c.OnscreensServerConfig) *rotator {
	return &rotator{
		cfg:           cfg,
		kind:          "right-rotator",
		freq:          rightRotatorUpdateFrequency,
		messages:      possibleRightMessages,
		promoMessages: promoRightMessages,
		liveLine:      rightLiveLine,
	}
}
