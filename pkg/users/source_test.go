package users

import "testing"

// noopChatterSource is a ChatterSource that reports nobody in chat and nobody
// followed/subscribed. Used by tests that build a *Sessions but don't exercise
// the source.
type noopChatterSource struct{}

func (noopChatterSource) UpdateChatters()               {}
func (noopChatterSource) Chatters() map[string]struct{} { return map[string]struct{}{} }
func (noopChatterSource) ChatterCount() int             { return 0 }
func (noopChatterSource) IsSubscriber(_ string) bool    { return false }
func (noopChatterSource) IsFollower(_ string) bool      { return false }

// recordingChatterSource answers from canned maps and counts calls, so tests
// can assert the seam actually routes through the injected source.
type recordingChatterSource struct {
	subscribers map[string]bool
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
func (r *recordingChatterSource) IsFollower(username string) bool {
	r.followCalls++
	return r.followers[username]
}

// Sessions.IsSubscriber routes through the injected ChatterSource — the seam a
// future YouTube/TikTok adapter swaps into (each provider gets its own
// *Sessions + source).
func TestSessionsIsSubscriberUsesChatterSource(t *testing.T) {
	rec := &recordingChatterSource{subscribers: map[string]bool{"alice": true}}
	s := New(rec)

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

// Two *Sessions hold independent state — the prerequisite for running a
// per-platform bot instance (e.g. YouTube) beside the Twitch one without them
// sharing a global session map.
func TestSessionsHoldIndependentState(t *testing.T) {
	a := New(noopChatterSource{})
	b := New(noopChatterSource{})

	a.lifetimeLeaderboard = [][]string{{"x", "1"}}
	a.loggedIn["x"] = &User{Username: "x"}

	if len(b.lifetimeLeaderboard) != 0 || len(b.loggedIn) != 0 {
		t.Fatalf("expected independent *Sessions, but b saw a's state: lb=%v loggedIn=%v",
			b.lifetimeLeaderboard, b.loggedIn)
	}
}
