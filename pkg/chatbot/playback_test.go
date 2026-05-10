package chatbot

import "testing"

func TestParseSkipParams(t *testing.T) {
	tests := []struct {
		name       string
		params     []string
		defaultN   int
		wantN      int
		wantOK     bool
	}{
		{"empty uses default", []string{}, 1, 1, true},
		{"empty uses default 5", []string{}, 5, 5, true},
		{"positive number", []string{"3"}, 1, 3, true},
		{"negative number", []string{"-30"}, 1, -30, true},
		{"zero", []string{"0"}, 1, 0, true},
		{"non-numeric rejected", []string{"abc"}, 1, 0, false},
		{"too many args rejected", []string{"1", "2"}, 1, 0, false},
		{"empty string rejected", []string{""}, 1, 0, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			n, ok := parseSkipParams(tc.params, tc.defaultN)
			if ok != tc.wantOK {
				t.Errorf("parseSkipParams(%v, %d) ok = %v, want %v", tc.params, tc.defaultN, ok, tc.wantOK)
			}
			if ok && n != tc.wantN {
				t.Errorf("parseSkipParams(%v, %d) n = %d, want %d", tc.params, tc.defaultN, n, tc.wantN)
			}
		})
	}
}
