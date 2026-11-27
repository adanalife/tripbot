package onscreensServer

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strings"
	"sync"
	"time"
)

// usStatesJSON is the outline of the lower 48 states and DC, as
// {"State": [[[lng, lat], ...], ...]} with one outer ring per landmass.
// Derived from the US Census Bureau's cartographic boundary files (public
// domain), rounded to 0.01°. Embedded so the map has no tile service or
// network fetch anywhere on the stream path.
//
//go:embed assets/us-states.json
var usStatesJSON []byte

// The minimap's frame: the lower 48 with a little margin, drawn as a plain
// equirectangular map whose width is squeezed by cos(37.5°) so the country
// keeps its familiar proportions. One linear projection serves both the
// outlines and the dot, so the two can't drift apart.
const (
	minimapWest, minimapEast   = -125.0, -66.5
	minimapSouth, minimapNorth = 24.5, 49.5
	minimapHeight              = 250.0
)

var minimapWidth = (minimapEast - minimapWest) / (minimapNorth - minimapSouth) *
	math.Cos(37.5*math.Pi/180) * minimapHeight

// project maps a coordinate into the minimap's drawing units. ok is false for
// anywhere outside the frame (Alaska, Canada, a 0,0 misread), which has no
// place on this map.
func project(lat, lng float64) (x, y float64, ok bool) {
	if lat < minimapSouth || lat > minimapNorth || lng < minimapWest || lng > minimapEast {
		return 0, 0, false
	}
	x = (lng - minimapWest) / (minimapEast - minimapWest) * minimapWidth
	y = (minimapNorth - lat) / (minimapNorth - minimapSouth) * minimapHeight
	return x, y, true
}

// statePath is one state's outline as SVG path data.
type statePath struct {
	name string
	d    string
}

// statePaths projects the embedded outlines once, in name order so the drawn
// map is byte-stable.
var statePaths = sync.OnceValue(func() []statePath {
	var states map[string][][][2]float64
	if err := json.Unmarshal(usStatesJSON, &states); err != nil {
		panic("onscreens: embedded us-states.json is malformed: " + err.Error())
	}
	paths := make([]statePath, 0, len(states))
	for name, rings := range states {
		var d strings.Builder
		for _, ring := range rings {
			for i, pt := range ring {
				// Unclamped: a coastline point just past the frame still draws.
				x := (pt[0] - minimapWest) / (minimapEast - minimapWest) * minimapWidth
				y := (minimapNorth - pt[1]) / (minimapNorth - minimapSouth) * minimapHeight
				cmd := "L"
				if i == 0 {
					cmd = "M"
				}
				fmt.Fprintf(&d, "%s%.1f %.1f", cmd, x, y)
			}
			d.WriteString("Z")
		}
		paths = append(paths, statePath{name: name, d: d.String()})
	}
	sort.Slice(paths, func(i, j int) bool { return paths[i].name < paths[j].name })
	return paths
})

// minimapFix is the last plotted position: where the dot sits, which state to
// light up, and when the feed reported it.
type minimapFix struct {
	x, y  float64
	state string
	at    time.Time
}

// key is the fix as the overlay's content. The browser source reloads the map
// image whenever it changes, so it carries everything the drawing depends on.
func (f minimapFix) key() string {
	return fmt.Sprintf("%.1f,%.1f,%s", f.x, f.y, f.state)
}

// newMinimap constructs the minimap *Onscreen.
func newMinimap() *Onscreen {
	slog.Info("creating onscreen", "kind", "minimap")
	return newOnscreen(defaultSleepInterval)
}

// plotMinimap records a position from the location feed and, when the minimap
// is switched on, shows it. A coordinate off the map hides the overlay rather
// than leaving the dot where the van used to be.
func (s *Server) plotMinimap(lat, lng float64, state string, now time.Time) {
	s.minimapMu.Lock()
	defer s.minimapMu.Unlock()
	x, y, ok := project(lat, lng)
	if !ok {
		s.minimapFix = nil
		s.Minimap.Hide()
		return
	}
	s.minimapFix = &minimapFix{x: x, y: y, state: state, at: now}
	if s.minimapOn {
		s.Minimap.Refresh(s.minimapFix.key(), locationDataTTL)
	}
}

// setMinimap switches the minimap on or off. Switching on shows the last
// position straight away when the feed reported one recently, instead of
// waiting up to a minute for the next.
func (s *Server) setMinimap(on bool, now time.Time) {
	s.minimapMu.Lock()
	defer s.minimapMu.Unlock()
	s.minimapOn = on
	if !on {
		s.Minimap.Hide()
		return
	}
	if f := s.minimapFix; f != nil {
		if left := locationDataTTL - now.Sub(f.at); left > 0 {
			s.Minimap.Refresh(f.key(), left)
		}
	}
}

// minimapSVG draws the map: every state, the one the van is in picked out, and
// a dot at the last position. With no position it is the bare map.
func (s *Server) minimapSVG() string {
	s.minimapMu.Lock()
	fix := s.minimapFix
	s.minimapMu.Unlock()

	var b strings.Builder
	const pad = 8.0
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="%.0f %.0f %.0f %.0f">`,
		-pad, -pad, minimapWidth+2*pad, minimapHeight+2*pad)
	fmt.Fprintf(&b, `<rect x="%.0f" y="%.0f" width="%.0f" height="%.0f" rx="14" fill="#0a0a0d" fill-opacity="0.55"/>`,
		-pad, -pad, minimapWidth+2*pad, minimapHeight+2*pad)
	b.WriteString(`<g stroke="#ffffff" stroke-opacity="0.7" stroke-width="0.7" stroke-linejoin="round">`)
	for _, p := range statePaths() {
		fill := `fill="#ffffff" fill-opacity="0.18"`
		if fix != nil && p.name == fix.state {
			fill = `fill="#ffd166" fill-opacity="0.75"`
		}
		fmt.Fprintf(&b, `<path %s d="%s"/>`, fill, p.d)
	}
	b.WriteString(`</g>`)
	if fix != nil {
		// The ring pulses so the dot reads as "live" at a glance.
		fmt.Fprintf(&b, `<circle cx="%.1f" cy="%.1f" r="6" fill="none" stroke="#ff3b30" stroke-width="2">`+
			`<animate attributeName="r" values="6;16" dur="2s" repeatCount="indefinite"/>`+
			`<animate attributeName="opacity" values="0.9;0" dur="2s" repeatCount="indefinite"/></circle>`,
			fix.x, fix.y)
		fmt.Fprintf(&b, `<circle cx="%.1f" cy="%.1f" r="5" fill="#ff3b30" stroke="#ffffff" stroke-width="1.5"/>`,
			fix.x, fix.y)
	}
	b.WriteString(`</svg>`)
	return b.String()
}
