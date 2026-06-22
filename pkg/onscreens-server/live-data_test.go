package onscreensServer

import (
	"testing"
	"time"
)

func poolHasText(pool []rotatorMessage, text string) bool {
	for _, m := range pool {
		if m.Text == text {
			return true
		}
	}
	return false
}

func TestLocationStoreFreshness(t *testing.T) {
	s := &locationStore{}
	now := time.Now()
	s.set("Moab, Utah", "Thursday June 14, 2018", now)

	if loc, date, ok := s.snapshot(now); !ok || loc != "Moab, Utah" || date != "Thursday June 14, 2018" {
		t.Errorf("fresh snapshot = (%q, %q, %v), want the set values + ok", loc, date, ok)
	}
	if _, _, ok := s.snapshot(now.Add(locationDataTTL + time.Minute)); ok {
		t.Error("snapshot past the TTL should not be ok (stale)")
	}
	if _, _, ok := (&locationStore{}).snapshot(now); ok {
		t.Error("an unset store should not be ok")
	}
}

func TestBotlessPoolsIncludeFreshLocationData(t *testing.T) {
	now := time.Now()
	liveLocation.set("Moab, Utah", "Thursday June 14, 2018", now)
	t.Cleanup(func() { liveLocation.set("", "", time.Time{}) })

	if got := botlessLeftPool(now); !poolHasText(got, "📍 Moab, Utah") {
		t.Errorf("bot-less left pool missing the location line: %+v", got)
	}
	if got := botlessRightPool(now); !poolHasText(got, "📅 Thursday June 14, 2018") {
		t.Errorf("bot-less right pool missing the date line: %+v", got)
	}
}

func TestBotlessPoolsOmitStaleData(t *testing.T) {
	now := time.Now()
	liveLocation.set("Moab, Utah", "Thursday June 14, 2018", now.Add(-locationDataTTL-time.Minute))
	t.Cleanup(func() { liveLocation.set("", "", time.Time{}) })

	// Stale data → pools fall back to the static promo sets only.
	left := botlessLeftPool(now)
	if poolHasText(left, "📍 Moab, Utah") {
		t.Error("stale location should not appear in the left pool")
	}
	if len(left) != len(botlessLeftMessages) {
		t.Errorf("left pool = %d entries, want the %d static promo lines", len(left), len(botlessLeftMessages))
	}
	if poolHasText(botlessRightPool(now), "📅 Thursday June 14, 2018") {
		t.Error("stale date should not appear in the right pool")
	}
}
