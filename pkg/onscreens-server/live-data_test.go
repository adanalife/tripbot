package onscreensServer

import (
	"strings"
	"testing"
	"time"

	rot "github.com/adanalife/tripbot/pkg/rotator"
)

// clipVars is a full set of clip data, as handleLocationUpdate builds from a
// location.update it just received.
var clipVars = rot.Vars{
	"location": "Moab, Utah",
	"state":    "Utah",
	"date":     "Thursday June 14, 2018",
	"weather":  "Clear sky, 88°F",
	"sunset":   "8:42 PM",
}

func poolHasText(pool []rot.Message, text string) bool {
	for _, m := range pool {
		if m.Text == text {
			return true
		}
	}
	return false
}

// stageClipData caches vars as of at and clears them when the test ends —
// liveLocation is package-scoped, so a test that left data behind would leak into
// the next one.
func stageClipData(t *testing.T, vars rot.Vars, at time.Time) {
	t.Helper()
	liveLocation.set(vars, at)
	t.Cleanup(func() { liveLocation.set(nil, time.Time{}) })
}

func TestLocationStoreFreshness(t *testing.T) {
	s := &locationStore{}
	now := time.Now()
	s.set(clipVars, now)

	if got := s.snapshot(now); got["location"] != "Moab, Utah" || got["weather"] != "Clear sky, 88°F" {
		t.Errorf("fresh snapshot = %v, want the set values", got)
	}
	if got := s.snapshot(now.Add(locationDataTTL + time.Minute)); got != nil {
		t.Errorf("snapshot past the TTL = %v, want nil (stale)", got)
	}
	if got := (&locationStore{}).snapshot(now); got != nil {
		t.Errorf("an unset store = %v, want nil", got)
	}
}

// The promo corners' live-data lines are written in $variables, so the pool always
// carries them and it's the pick that drops them when the data isn't there. This
// covers them rendering substituted when it is.
func TestPromoRotatorsRenderLiveClipData(t *testing.T) {
	cfg := rotatorConf(platformYouTube, false) // bot-less, so pool() returns the promo set
	stageClipData(t, clipVars, time.Now())

	if got := currentPool(leftRotator(cfg)); !poolHasText(got, "📍 $location") {
		t.Errorf("promo left pool missing the location line: %+v", got)
	}
	if got := currentPool(rightRotator(cfg)); !poolHasText(got, "📅 $date") {
		t.Errorf("promo right pool missing the date line: %+v", got)
	}

	var sawLocation, sawDate bool
	for i := 0; i < 4000 && !(sawLocation && sawDate); i++ {
		if leftRotator(cfg).content() == "📍 Moab, Utah" {
			sawLocation = true
		}
		if rightRotator(cfg).content() == "📅 Thursday June 14, 2018" {
			sawDate = true
		}
	}
	if !sawLocation || !sawDate {
		t.Errorf("expected both live lines to render substituted: location=%v date=%v", sawLocation, sawDate)
	}
}

func TestStaleClipDataDropsTheLiveLines(t *testing.T) {
	cfg := rotatorConf(platformYouTube, false)
	stageClipData(t, clipVars, time.Now().Add(-locationDataTTL-time.Minute))

	for i := 0; i < 2000; i++ {
		if got := leftRotator(cfg).content(); strings.Contains(got, "$") || strings.Contains(got, "Moab") {
			t.Fatalf("stale data leaked into the left corner: %q", got)
		}
		if got := rightRotator(cfg).content(); strings.Contains(got, "$") || strings.Contains(got, "2018") {
			t.Fatalf("stale data leaked into the right corner: %q", got)
		}
	}
}

// An authored line — the console-edited kind, in either pool on any platform —
// resolves the same variables the built-in live lines do.
func TestAuthoredVariableLineRendersOnAnyPlatform(t *testing.T) {
	stageClipData(t, clipVars, time.Now())

	l := leftRotator(rotatorConf(platformTwitch, true))
	l.setCopy(rot.Corner{Messages: []rot.Message{{Text: "$weather in $state, sunset $sunset"}}}, "")

	for i := 0; i < 200; i++ {
		if got := l.content(); got != "Clear sky, 88°F in Utah, sunset 8:42 PM" {
			t.Fatalf("content() = %q, want the substituted line", got)
		}
	}
}

// A corner whose only line needs data that hasn't arrived shows nothing rather
// than a literal token.
func TestUnresolvedAuthoredLineRendersNothing(t *testing.T) {
	l := leftRotator(rotatorConf(platformTwitch, true))
	l.setCopy(rot.Corner{Messages: []rot.Message{{Text: "$weather right now"}}}, "")

	if got := l.content(); got != "" {
		t.Errorf("content() = %q, want empty with no clip data", got)
	}
}

// The rare line takes variables too, and skips its roll rather than firing on a
// line it can't render — a 1-in-10000 easter egg is not worth spending on a bare
// token.
func TestRareLineSkipsWhenUnresolved(t *testing.T) {
	l := leftRotator(rotatorConf(platformTwitch, true))
	l.setCopy(rot.Corner{Messages: []rot.Message{{Text: "static"}}}, "rare in $state!")

	for i := 0; i < 2000; i++ {
		if got := l.content(); got != "static" {
			t.Fatalf("content() = %q, want only the static line with no clip data", got)
		}
	}
}
