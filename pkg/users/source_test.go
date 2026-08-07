package users

import (
	"testing"
	"time"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
)

// testConf is the config test Sessions carry — the same values .env.testing
// supplies, as a literal so tests don't reach through the loaded global.
var testConf = &c.TripbotConfig{
	Environment: "testing",
	Platform:    "twitch",
	ChannelName: "test",
}

// noopChatterSource is a ChatterSource that reports nobody in chat and nobody
// followed/subscribed. Used by tests that build a *Sessions but don't exercise
// the source.
type noopChatterSource struct{}

func (noopChatterSource) UpdateChatters()               {}
func (noopChatterSource) Chatters() map[string]struct{} { return map[string]struct{}{} }
func (noopChatterSource) ChatterCount() int             { return 0 }
func (noopChatterSource) IsSubscriber(_ string) bool    { return false }
func (noopChatterSource) SubscriberTier(_ string) int   { return 0 }
func (noopChatterSource) IsFollower(_ string) bool      { return false }

// recordingChatterSource answers from canned maps and counts calls, so tests
// can assert the seam actually routes through the injected source.
type recordingChatterSource struct {
	subscribers map[string]bool
	tiers       map[string]int
	followers   map[string]bool
	subCalls    int
	followCalls int
}

func (recordingChatterSource) UpdateChatters()               {}
func (recordingChatterSource) Chatters() map[string]struct{} { return map[string]struct{}{} }
func (recordingChatterSource) ChatterCount() int             { return 0 }
func (r *recordingChatterSource) IsSubscriber(username string) bool {
	r.subCalls++
	return r.subscribers[username]
}
func (r *recordingChatterSource) SubscriberTier(username string) int {
	return r.tiers[username]
}
func (r *recordingChatterSource) IsFollower(username string) bool {
	r.followCalls++
	return r.followers[username]
}

// Sessions.IsSubscriber routes through the injected ChatterSource — the seam a
// future YouTube/TikTok adapter swaps into (each provider gets its own
// *Sessions + source).
func TestSessionsIsSubscriberUsesChatterSource(t *testing.T) {
	rec := &recordingChatterSource{subscribers: map[string]bool{"alice": true}}
	s := New(testConf, rec)

	if !s.IsSubscriber(User{Username: "alice"}) {
		t.Fatal("expected alice to be a subscriber via the injected source")
	}
	if s.IsSubscriber(User{Username: "bob"}) {
		t.Fatal("expected bob not to be a subscriber")
	}
	if rec.subCalls != 2 {
		t.Fatalf("expected 2 IsSubscriber calls on the source, got %d", rec.subCalls)
	}
}

// BonusMiles scales with the subscription tier from the ChatterSource: 5% of
// session miles per tier, with non-subscribers (tier 0) shown the tier-1 rate.
func TestBonusMilesScalesWithSubscriberTier(t *testing.T) {
	rec := &recordingChatterSource{tiers: map[string]int{"tier1": 1, "tier2": 2, "tier3": 3}}
	s := New(testConf, rec)

	loginTime := time.Now().Add(-3 * time.Minute) // ≈0.1 session miles
	for _, username := range []string{"nonsub", "tier1", "tier2", "tier3"} {
		s.loggedIn[username] = &User{Username: username, LoggedIn: loginTime}
	}

	base := s.BonusMiles(User{Username: "tier1"})
	if base <= 0 {
		t.Fatalf("expected a positive tier-1 bonus, got %f", base)
	}
	tests := []struct {
		username string
		want     float32
	}{
		{"nonsub", base}, // would-be bonus at the tier-1 rate
		{"tier2", base * 2},
		{"tier3", base * 3},
	}
	for _, tt := range tests {
		got := s.BonusMiles(User{Username: tt.username})
		// the sessions were logged in at the same instant, but the two
		// BonusMiles calls happen a hair apart — allow a tiny drift
		if diff := got - tt.want; diff < -0.001 || diff > 0.001 {
			t.Errorf("BonusMiles(%q) = %f, want ≈%f", tt.username, got, tt.want)
		}
	}
}

// Two *Sessions hold independent state — the prerequisite for running a
// per-platform bot instance (e.g. YouTube) beside the Twitch one without them
// sharing a global session map.
func TestSessionsHoldIndependentState(t *testing.T) {
	a := New(testConf, noopChatterSource{})
	b := New(testConf, noopChatterSource{})

	a.lifetimeLeaderboard = [][]string{{"x", "1"}}
	a.loggedIn["x"] = &User{Username: "x"}

	if len(b.lifetimeLeaderboard) != 0 || len(b.loggedIn) != 0 {
		t.Fatalf("expected independent *Sessions, but b saw a's state: lb=%v loggedIn=%v",
			b.lifetimeLeaderboard, b.loggedIn)
	}
}
