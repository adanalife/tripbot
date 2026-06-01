package server

import (
	"testing"

	"github.com/adanalife/tripbot/pkg/feature"
)

// New gives a Server with sensible default runtime state.
func TestNewServerDefaults(t *testing.T) {
	s := New()
	if s.hub == nil {
		t.Fatal("New() left hub nil")
	}
	if s.versionTag != "dev" {
		t.Errorf("versionTag = %q, want %q", s.versionTag, "dev")
	}
	if s.flagClient == nil {
		t.Fatal("New() left flagClient nil")
	}
	if s.TwitchConnected() {
		t.Error("twitchConnected should default to false")
	}
}

// The setters mutate the instance they're called on, and two Servers hold
// independent runtime state.
func TestServerSettersAreInstanceScoped(t *testing.T) {
	a := New()
	b := New()

	a.SetVersion("v1.2.3")
	a.SetTwitchConnected(true)
	a.SetFlagClient(feature.NewInMemoryClient(map[string]feature.Flag{
		"demo": {Key: "demo"},
	}))

	if b.versionTag != "dev" || b.TwitchConnected() {
		t.Errorf("b picked up a's state: version=%q connected=%v", b.versionTag, b.TwitchConnected())
	}
	if got := a.flagSnapshot(t.Context()); len(got) != 1 {
		t.Errorf("a.flagSnapshot = %d flags, want 1", len(got))
	}
	if got := b.flagSnapshot(t.Context()); len(got) != 0 {
		t.Errorf("b.flagSnapshot = %d flags, want 0 (independent of a)", len(got))
	}
}
