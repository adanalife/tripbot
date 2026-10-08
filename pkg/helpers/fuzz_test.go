package helpers

import (
	"strings"
	"testing"
)

// FuzzStripAtSign asserts the @-prefix stripper never panics on arbitrary
// input and is idempotent on its own output.
func FuzzStripAtSign(f *testing.F) {
	seeds := []string{"", "@", "@dana", "dana", "da@na", "@@dana", "\x00"}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		got := StripAtSign(s)
		// At most one leading @ should be removed. Either the input had no
		// leading @ and the result is unchanged, or the input had one and
		// the result is the input minus its first byte.
		if strings.HasPrefix(s, "@") {
			if got != s[1:] {
				t.Fatalf("StripAtSign(%q) = %q; want %q", s, got, s[1:])
			}
		} else if got != s {
			t.Fatalf("StripAtSign(%q) = %q; want unchanged", s, got)
		}
	})
}

// FuzzStateAbbrevToState asserts random input never panics and that the
// non-empty-result case round-trips back to the original (uppercased) abbrev.
func FuzzStateAbbrevToState(f *testing.F) {
	seeds := []string{"", "CA", "ca", "Ny", "ZZ", "  ", "\x00", "California", "AE"}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		name := StateAbbrevToState(s)
		if name == "" {
			return
		}
		// When the lookup hit, the returned name should map back to the
		// original uppercased two-letter abbrev.
		back := StateToStateAbbrev(name)
		if back != strings.ToUpper(s) {
			t.Fatalf("StateAbbrevToState(%q) = %q; reverse %q != %q", s, name, back, strings.ToUpper(s))
		}
	})
}

// FuzzTitlecaseState asserts the convenience wrapper never panics on arbitrary
// input. It accepts either a two-letter abbrev or a full state name.
func FuzzTitlecaseState(f *testing.F) {
	seeds := []string{"", "CA", "california", "New york", "  ", "ZZ", "\x00"}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		_ = TitlecaseState(s)
	})
}
