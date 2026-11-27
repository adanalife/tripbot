package onscreensServer

// Slug identifiers for the onscreens that onscreens-server's HTTP layer addresses
// individually (state endpoint, render template, asset). Names match the URL
// path components OBS browser sources fetch.
const (
	SlugMiddleText   = "middle-text"
	SlugLeaderboard  = "leaderboard"
	SlugLeftMessage  = "left-message"
	SlugRightMessage = "right-message"
	SlugTimewarp     = "timewarp"
	SlugGPS          = "gps"
	// SlugWeather has no *Onscreen behind it: state.json derives it from the
	// location feed's cached $weather (see weatherView), so it shows and hides
	// with the same freshness the corner rotators use.
	SlugWeather = "weather"
	SlugMinimap = "minimap"
)

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
		SlugMinimap:      s.Minimap,
	}
}

// graduated is the set of overlays a production stream shows. An overlay
// outside it renders everywhere except production, so a new one can be judged
// on stage before it reaches a live audience; adding its slug here is what
// ships it.
var graduated = map[string]bool{
	SlugMiddleText:   true,
	SlugLeaderboard:  true,
	SlugLeftMessage:  true,
	SlugRightMessage: true,
	SlugTimewarp:     true,
	SlugGPS:          true,
}

// hideUngraduated blanks every overlay in views that production doesn't show
// yet. state.json is the one feed every overlay page draws from, so masking it
// here covers an overlay whatever computes its state.
func hideUngraduated(views map[string]onscreenView) {
	for slug := range views {
		if !graduated[slug] {
			views[slug] = onscreenView{}
		}
	}
}
