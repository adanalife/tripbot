package instrumentation

import "testing"

// CurrentState.Set must track exactly one active state at a time: a blank
// abbrev normalizes to "unknown", a transition advances prev to the new
// state, and a repeated Set of the same state is a no-op. The OTel gauge
// values themselves aren't read back here (no reader is wired in unit tests);
// the prev bookkeeping is the load-bearing "clear the old =1 series" logic, so
// that's what we assert.
func TestCurrentStateSet_TracksActiveState(t *testing.T) {
	s := &currentStateIface{gauge: currentState}

	tests := []struct {
		name     string
		in       string
		wantPrev string
	}{
		{"first set", "MO", "MO"},
		{"transition", "KS", "KS"},
		{"repeat is no-op", "KS", "KS"},
		{"blank normalizes to unknown", "", "unknown"},
		{"recover from unknown", "CO", "CO"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s.Set(tt.in)
			if s.prev != tt.wantPrev {
				t.Errorf("after Set(%q), prev = %q, want %q", tt.in, s.prev, tt.wantPrev)
			}
		})
	}
}

// The package-level CurrentState must be usable as a no-config no-op (the
// default OTel meter swallows records), matching how every other iface in
// this package behaves when no exporter is configured.
func TestCurrentStateSet_DefaultIsSafe(t *testing.T) {
	CurrentState.Set("WY")
	CurrentState.Set("")
}
