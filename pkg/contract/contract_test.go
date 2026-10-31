package contract

import (
	_ "embed"
	"testing"
)

//go:embed contract.json
var committed []byte

// TestContractMatchesCommitted is the anti-drift guard: it regenerates the
// contract from the in-code constants and asserts the result is byte-identical
// to the committed contract.json. If a service name, port, or env-var key
// changes in contract.go without regenerating, this fails CI.
func TestContractMatchesCommitted(t *testing.T) {
	got, err := Current().Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	if string(got) != string(committed) {
		t.Errorf("contract.json is out of date with pkg/contract constants.\n"+
			"Run `go generate ./pkg/contract` and commit the result.\n\n"+
			"--- committed contract.json ---\n%s\n--- generated from constants ---\n%s",
			committed, got)
	}
}
