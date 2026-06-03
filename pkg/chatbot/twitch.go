package chatbot

import (
	"time"

	mytwitch "github.com/adanalife/tripbot/pkg/twitch"
)

// Twitch is the command-time Twitch Helix surface the chatbot needs. Today
// that's only follow lookups (followageCmd); it grows as admin-panel Helix
// features land (ban/timeout, send-as-broadcaster — see
// vault/tripbot/tripbot/TODO.md).
//
// It is deliberately the seam where, once the Twitch Helix API moves into its
// own service (vault/tripbot/TODO.md "Extract Twitch Helix API into a separate
// service"), the in-process adapter below is swapped for an HTTP client —
// without touching any command code.
type Twitch interface {
	FollowedAt(username string) (time.Time, bool)
}

// realTwitch is the production adapter the App is wired with in New().
// Delegates to the package-level pkg/twitch shim (defaultClient). Mirrors the
// realVLC / realOnscreens shape.
type realTwitch struct{}

func (realTwitch) FollowedAt(username string) (time.Time, bool) {
	return mytwitch.FollowedAt(username)
}
