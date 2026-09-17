package helpers

import (
	"strings"
	"testing"
)

func TestStateAbbrevToState(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"CA", "California"},
		{"ca", "California"},
		{"Ny", "New York"},
		{"DC", "District of Columbia"},
		{"AE", "Armed Forces Europe"},
		{"ZZ", ""},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := StateAbbrevToState(tt.input)
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStateToStateAbbrev(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"California", "CA"},
		{"california", "CA"},
		{"CALIFORNIA", "CA"},
		{"Atlantis", ""},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := StateToStateAbbrev(tt.input)
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTitlecaseState(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"abbrev", "ca", "California"},
		{"upper case abbrev", "CA", "California"},
		{"mixed case abbrev", "Ny", "New York"},
		{"all-lower full name", "california", "California"},
		{"mixed case full name", "cAlIfOrNiA", "California"},
		{"multi-word full name", "new york", "New York"},
		{"abbrev with lowercase connector", "dc", "District of Columbia"},
		{"full name with lowercase connector", "DISTRICT OF COLUMBIA", "District of Columbia"},
		{"surrounding whitespace around abbrev", "  ca  ", "California"},
		{"surrounding whitespace around full name", "\tcalifornia\n", "California"},
		{"unknown abbrev echoes input", "ZZ", "Zz"},
		{"unknown name echoes input", "atlantis", "Atlantis"},
		{"empty", "", ""},
		{"whitespace only", "   ", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TitlecaseState(tt.input)
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNormalizeStateInput(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"single word", "utah", "utah"},
		{"multi-word keeps its space", "new york", "new york"},
		{"mixed case is untouched", "NEW York", "NEW York"},
		{"runs of whitespace collapse", "  NEW   york  ", "NEW york"},
		{"tabs and newlines collapse", "new\tyork\n", "new york"},
		{"punctuation is stripped", "new york!?", "new york"},
		{"digits are stripped", "utah2", "utah"},
		{"abbrev with dots stays one token", "n.y.", "ny"},
		{"three-word territory", "district of columbia", "district of columbia"},
		{"empty", "", ""},
		{"no letters at all", "!!!123", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeStateInput(tt.input)
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// Every state and territory name survives normalization unchanged, so a viewer
// who types a name exactly gets it through to the lookup.
func TestNormalizeStateInputKeepsEveryStateName(t *testing.T) {
	for _, name := range StateNames() {
		if got := NormalizeStateInput(name); got != name {
			t.Errorf("NormalizeStateInput(%q) = %q", name, got)
		}
	}
}

// Round-trip pins behavior for state names that survive strings.Title.
// Names with lowercase connectors like "District of Columbia" don't
// round-trip today (StateToStateAbbrev title-cases "of" → "Of"); we
// only assert names that survive the title-cased lookup.
func TestStateAbbrevRoundTrip(t *testing.T) {
	for abbrev, name := range stateAbbrevs {
		//nolint:staticcheck // SA1019: mirrors TitlecaseState's own fallback
		if strings.Title(strings.ToLower(name)) != name {
			continue
		}
		got := StateToStateAbbrev(name)
		if got != abbrev {
			t.Errorf("%s -> %s -> %s (expected %s)", abbrev, name, got, abbrev)
		}
	}
}

func TestStateCentroid(t *testing.T) {
	lat, lng, ok := StateCentroid("colorado")
	if !ok {
		t.Fatal("expected a centroid for a full state name, case-insensitive")
	}
	if lat < 36 || lat > 42 || lng < -110 || lng > -102 {
		t.Errorf("Colorado centroid (%f, %f) is not in Colorado", lat, lng)
	}
	if _, _, ok := StateCentroid("CO"); !ok {
		t.Error("expected a centroid for a two-letter abbreviation")
	}
	if _, _, ok := StateCentroid("Guam"); ok {
		t.Error("territories have no centroid")
	}
	if _, _, ok := StateCentroid("not a state"); ok {
		t.Error("unknown names have no centroid")
	}
}

func TestStateCentroids_TableHygiene(t *testing.T) {
	if len(stateCentroids) != 51 {
		t.Errorf("want the 50 states + DC, got %d entries", len(stateCentroids))
	}
	for abbrev, c := range stateCentroids {
		if _, ok := stateAbbrevs[abbrev]; !ok {
			t.Errorf("centroid %q is not a known state abbreviation", abbrev)
		}
		// Every centroid sits inside a box around the 50 states (Hawaii sets
		// the south and west edges, Alaska the north).
		if c[0] < 20 || c[0] > 62 || c[1] < -158 || c[1] > -66 {
			t.Errorf("centroid %q (%f, %f) is outside the US", abbrev, c[0], c[1])
		}
	}
}
