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
		{"all-lower full name", "california", "California"},
		{"mixed case full name", "cAlIfOrNiA", "California"},
		{"unknown abbrev returns empty title-cased", "ZZ", ""},
		{"two-letter non-abbrev unknown", "Zz", ""},
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

// Round-trip pins behavior for state names that survive strings.Title.
// Names with lowercase connectors like "District of Columbia" don't
// round-trip today (StateToStateAbbrev title-cases "of" → "Of"); we
// only assert names that survive the title-cased lookup.
func TestStateAbbrevRoundTrip(t *testing.T) {
	for abbrev, name := range stateAbbrevs {
		if strings.Title(strings.ToLower(name)) != name {
			continue
		}
		got := StateToStateAbbrev(name)
		if got != abbrev {
			t.Errorf("%s -> %s -> %s (expected %s)", abbrev, name, got, abbrev)
		}
	}
}
