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
	withPlatform(t, platformYouTube)
	withInbound(t, false) // bot-less, so pool() returns the promo set
	now := time.Now()
	liveLocation.set("Moab, Utah", "Thursday June 14, 2018", now)
	t.Cleanup(func() { liveLocation.set("", "", time.Time{}) })

	if got := newLeftRotator().pool(now); !poolHasText(got, "📍 Moab, Utah") {
		t.Errorf("bot-less left pool missing the location line: %+v", got)
	}
	if got := newRightRotator().pool(now); !poolHasText(got, "📅 Thursday June 14, 2018") {
		t.Errorf("bot-less right pool missing the date line: %+v", got)
	}
}

func TestBotlessPoolsOmitStaleData(t *testing.T) {
	withPlatform(t, platformYouTube)
	withInbound(t, false)
	now := time.Now()
	liveLocation.set("Moab, Utah", "Thursday June 14, 2018", now.Add(-locationDataTTL-time.Minute))
	t.Cleanup(func() { liveLocation.set("", "", time.Time{}) })

	// Stale data → pools fall back to the static promo sets only.
	left := newLeftRotator().pool(now)
	if poolHasText(left, "📍 Moab, Utah") {
		t.Error("stale location should not appear in the left pool")
	}
	if len(left) != len(botlessLeftMessages) {
		t.Errorf("left pool = %d entries, want the %d static promo lines", len(left), len(botlessLeftMessages))
	}
	if poolHasText(newRightRotator().pool(now), "📅 Thursday June 14, 2018") {
		t.Error("stale date should not appear in the right pool")
	}
}

// TestLiveLineFreshness covers the live-data line builders directly: fresh data
// yields a weighted line, stale/empty data yields ok=false.
func TestLiveLineFreshness(t *testing.T) {
	now := time.Now()
	liveLocation.set("Moab, Utah", "Thursday June 14, 2018", now)
	t.Cleanup(func() { liveLocation.set("", "", time.Time{}) })

	if line, ok := leftLiveLine(now); !ok || line.Text != "📍 Moab, Utah" {
		t.Errorf("leftLiveLine(fresh) = (%q, %v), want the location line + ok", line.Text, ok)
	}
	if line, ok := rightLiveLine(now); !ok || line.Text != "📅 Thursday June 14, 2018" {
		t.Errorf("rightLiveLine(fresh) = (%q, %v), want the date line + ok", line.Text, ok)
	}
	if _, ok := leftLiveLine(now.Add(locationDataTTL + time.Minute)); ok {
		t.Error("leftLiveLine(stale) should not be ok")
	}
}
