package contract

import (
	_ "embed"
	"testing"
)

//go:embed eventbus.json
var committedEventbus []byte

// TestEventbusMatchesCommitted is the anti-drift guard for the eventbus
// registry: it regenerates eventbus.json from the in-code declaration and
// asserts the result is byte-identical to the committed file. If the registry
// changes without regenerating, this fails CI.
func TestEventbusMatchesCommitted(t *testing.T) {
	got, err := MarshalEventbus()
	if err != nil {
		t.Fatalf("MarshalEventbus: %v", err)
	}
	if string(got) != string(committedEventbus) {
		t.Errorf("eventbus.json is out of date with the pkg/contract eventbus registry.\n"+
			"Run `go generate ./pkg/contract` and commit the result.\n\n"+
			"--- committed eventbus.json ---\n%s\n--- generated ---\n%s",
			committedEventbus, got)
	}
}
