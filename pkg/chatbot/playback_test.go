package chatbot

import (
	"strings"
	"testing"
)

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

func TestFormatSkipReply(t *testing.T) {
	tests := []struct {
		name    string
		count   int
		forward bool
		want    string
	}{
		{"single forward", 1, true, "Skipped 1 video forward!"},
		{"single back", 1, false, "Skipped 1 video back!"},
		{"plural forward", 5, true, "Skipped 5 videos forward!"},
		{"plural back", 3, false, "Skipped 3 videos back!"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := formatSkipReply(tc.count, tc.forward)
			if got != tc.want {
				t.Errorf("formatSkipReply(%d, %v) = %q, want %q", tc.count, tc.forward, got, tc.want)
			}
			// concise enough for Twitch's 500-char limit (with headroom)
			if len(got) > 100 {
				t.Errorf("reply too long for chat: %d chars", len(got))
			}
			if !strings.HasPrefix(got, "Skipped ") {
				t.Errorf("reply does not start with 'Skipped ': %q", got)
			}
		})
	}
}
