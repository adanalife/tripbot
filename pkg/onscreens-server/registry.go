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
	// SlugUnderConstruction renders a static full-screen slate that sits
	// *beneath* the dashcam video in the OBS scene stack: it becomes visible
	// only when the dashcam source has no frames to composite (playout
	// restart, RTSP drop). It has no *Onscreen state and no show/hide — the
	// scene layering is the visibility mechanism — so it appears in the
	// render registry but not in all().
	SlugUnderConstruction = "under-construction"
)

// Lookup returns the *Onscreen registered under slug on this *Server, or
// nil if unknown. The returned pointer is the live singleton — Show/Hide
// on it mutates in place.
func (s *Server) Lookup(slug string) *Onscreen {
	return s.all()[slug]
}

// Snapshot returns a fresh map of every registered onscreen keyed by slug.
// The *Onscreen pointers are live; mutating one is visible everywhere.
// Used by callers that need the full set at once (e.g. /onscreens/state.json).
func (s *Server) Snapshot() map[string]*Onscreen {
	return s.all()
}

func (s *Server) all() map[string]*Onscreen {
	return map[string]*Onscreen{
		SlugMiddleText:   s.MiddleText,
		SlugLeaderboard:  s.Leaderboard,
		SlugLeftMessage:  s.LeftRotator,
		SlugRightMessage: s.RightRotator,
		SlugTimewarp:     s.Timewarp,
		SlugGPS:          s.GPS,
	}
}
