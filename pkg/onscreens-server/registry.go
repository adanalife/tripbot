package onscreensServer

// Slug identifiers for the onscreens that vlc-server's HTTP layer addresses
// individually (state endpoint, render template, asset). Names match the URL
// path components OBS browser sources fetch.
const (
	SlugMiddleText   = "middle-text"
	SlugLeaderboard  = "leaderboard"
	SlugLeftMessage  = "left-message"
	SlugRightMessage = "right-message"
	SlugTimewarp     = "timewarp"
	SlugGPS          = "gps"
	SlugFlag         = "flag"
)

// Lookup returns the Onscreen registered under slug, or nil if unknown.
// The returned pointer is the live singleton — Show/Hide on it mutates
// in place.
func Lookup(slug string) *Onscreen {
	return all()[slug]
}

// Snapshot returns a fresh map of every registered onscreen keyed by slug.
// The *Onscreen pointers are live; mutating one is visible everywhere.
// Used by callers that need the full set at once (e.g. /onscreens/state.json).
func Snapshot() map[string]*Onscreen {
	return all()
}

func all() map[string]*Onscreen {
	return map[string]*Onscreen{
		SlugMiddleText:   middleText,
		SlugLeaderboard:  leaderboard,
		SlugLeftMessage:  leftRotator,
		SlugRightMessage: rightRotator,
		SlugTimewarp:     timewarp,
		SlugGPS:          gpsImage,
		SlugFlag:         flagImage,
	}
}
