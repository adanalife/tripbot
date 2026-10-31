package contract

import (
	_ "embed"
	"testing"
)

//go:embed commands.json
var committedCommands []byte

// TestCommandsMatchesCommitted is the anti-drift guard for the onscreens command
// registry: it regenerates commands.json from the in-code declaration and
// asserts the result is byte-identical to the committed file. If the registry
// changes without regenerating, this fails CI.
func TestCommandsMatchesCommitted(t *testing.T) {
	got, err := MarshalCommands()
	if err != nil {
		t.Fatalf("MarshalCommands: %v", err)
	}
	if string(got) != string(committedCommands) {
		t.Errorf("commands.json is out of date with the pkg/contract command registry.\n"+
			"Run `go generate ./pkg/contract` and commit the result.\n\n"+
			"--- committed commands.json ---\n%s\n--- generated ---\n%s",
			committedCommands, got)
	}
}
