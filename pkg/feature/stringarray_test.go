package feature

import (
	"reflect"
	"testing"
)

// The elements that make an array literal more than a comma-join: a space and a
// comma force quoting, a quote and a backslash force escaping, and an empty
// string is only distinguishable from no elements by its quotes. Round-tripped
// rather than compared against a hand-written literal, since the literal is
// pgx's business — what this asserts is that nothing is lost either way.
func TestStringArrayRoundTrip(t *testing.T) {
	for _, in := range []stringArray{
		{},
		{"dana"},
		{"dana", "someone_else"},
		{"some one", "a,b", `wi"th`, `back\slash`, ""},
	} {
		v, err := in.Value()
		if err != nil {
			t.Fatalf("Value(%q): %v", in, err)
		}
		var out stringArray
		if err := out.Scan(v); err != nil {
			t.Fatalf("Scan(%v): %v", v, err)
		}
		if !reflect.DeepEqual([]string(in), []string(out)) {
			t.Errorf("round trip: got %#v, want %#v (literal %v)", out, in, v)
		}
	}
}

// A nil array is SQL NULL in both directions — the case the allowlist columns
// reject, so it has to stay distinguishable from an empty array rather than
// being silently coerced into one.
func TestStringArrayNull(t *testing.T) {
	v, err := stringArray(nil).Value()
	if err != nil || v != nil {
		t.Fatalf("nil Value(): got %v, %v; want nil, nil", v, err)
	}
	out := stringArray{"stale"}
	if err := out.Scan(nil); err != nil {
		t.Fatalf("Scan(nil): %v", err)
	}
	if out != nil {
		t.Errorf("Scan(nil): got %#v, want nil", out)
	}
}

func TestStringArrayScanRejectsOtherTypes(t *testing.T) {
	var out stringArray
	if err := out.Scan(42); err == nil {
		t.Error("Scan(int): want an error, got nil")
	}
}
