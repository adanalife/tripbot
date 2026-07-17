package helpers

import (
	"strings"
	"testing"
	"time"
)

func TestParseLatLng(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantLat   float64
		wantLon   float64
		wantErr   bool
		errSubstr string
	}{
		{
			name:    "valid USA coords (Salt Lake City area)",
			input:   "W111.845329N40.774768",
			wantLat: 40.774768,
			wantLon: -111.845329,
			wantErr: false,
		},
		{
			name:      "missing N character",
			input:     "W111.845329S40.774768",
			wantErr:   true,
			errSubstr: "can't find an N",
		},
		{
			name:      "N is the first letter",
			input:     "N40.774768",
			wantErr:   true,
			errSubstr: "N was the first letter",
		},
		{
			name:      "garbage that parses to zero",
			input:     "WxxxNyyy",
			wantErr:   true,
			errSubstr: "failed to convert",
		},
		{
			name:      "lat outside USA bounds",
			input:     "W120.0N10.0",
			wantErr:   true,
			errSubstr: "outside USA",
		},
		{
			name:      "lon outside USA bounds (too far west)",
			input:     "W175.0N40.0",
			wantErr:   true,
			errSubstr: "outside USA",
		},
		{
			name:      "lat above 90 (impossible)",
			input:     "W120.0N91.0",
			wantErr:   true,
			errSubstr: "impossible magnitude",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lat, lon, err := ParseLatLng(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil (lat=%v lon=%v)", tt.errSubstr, lat, lon)
				}
				if !strings.Contains(err.Error(), tt.errSubstr) {
					t.Fatalf("expected error containing %q, got %q", tt.errSubstr, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if lat != tt.wantLat || lon != tt.wantLon {
				t.Fatalf("got (%v, %v), want (%v, %v)", lat, lon, tt.wantLat, tt.wantLon)
			}
		})
	}
}

func TestDurationToMiles(t *testing.T) {
	tests := []struct {
		name string
		dur  time.Duration
		want float32
	}{
		{"zero duration", 0, 0},
		{"3 minutes = 0.1 miles", 3 * time.Minute, 0.1},
		{"30 minutes = 1 mile", 30 * time.Minute, 1.0},
		{"1 hour = 2 miles", time.Hour, 2.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DurationToMiles(tt.dur)
			diff := got - tt.want
			if diff < 0 {
				diff = -diff
			}
			if diff > 1e-5 {
				t.Fatalf("got %v, want %v (diff=%v)", got, tt.want, diff)
			}
		})
	}
}

func TestGoogleMapsURL(t *testing.T) {
	got := GoogleMapsURL(40.774768, -111.845329)
	want := "https://maps.google.com/?q=40.77477%2C-111.84533&ll=40.77477%2C-111.84533&z=5"
	if got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
}

func TestRemoveNonLetters(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty", "", ""},
		{"all letters", "Hello", "Hello"},
		{"mixed", "Hello, World! 123", "HelloWorld"},
		{"only punctuation", "!!!", ""},
		{"unicode (non-ascii) is stripped", "café", "caf"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RemoveNonLetters(tt.input)
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStripAtSign(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"with @ prefix", "@dana", "dana"},
		{"without @ prefix", "dana", "dana"},
		{"@ in middle is preserved", "da@na", "da@na"},
		{"only @ sign", "@", ""},
		{"empty string", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StripAtSign(tt.input)
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestActualDate(t *testing.T) {
	utc := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	got := ActualDate(utc, 40.7128, -74.0060)
	if got.Location().String() != "America/New_York" {
		t.Fatalf("expected America/New_York timezone, got %s", got.Location().String())
	}
	if !got.Equal(utc) {
		t.Fatalf("instant changed: got %v, want %v", got, utc)
	}
}

func TestIsDaytime(t *testing.T) {
	lat, lon := 40.7128, -74.0060 // New York (EDT in July, UTC-4)

	// 12:00 UTC on July 4 is 08:00 EDT — well after sunrise, well before sunset.
	if !IsDaytime(time.Date(2024, 7, 4, 12, 0, 0, 0, time.UTC), lat, lon) {
		t.Error("expected 08:00 EDT to be daytime")
	}
	// 02:00 UTC on July 5 is 22:00 EDT on July 4 — after sunset.
	if IsDaytime(time.Date(2024, 7, 5, 2, 0, 0, 0, time.UTC), lat, lon) {
		t.Error("expected 22:00 EDT to be nighttime")
	}
}

func TestLocalDate(t *testing.T) {
	lat, lon := 40.7128, -74.0060 // New York (EDT in July, UTC-4)

	// 02:00 UTC on July 5 is 22:00 EDT on July 4 — the local calendar day is
	// the 4th, not the 5th the UTC instant falls on.
	got := LocalDate(time.Date(2024, 7, 5, 2, 0, 0, 0, time.UTC), lat, lon)
	if y, m, d := got.Date(); y != 2024 || m != time.July || d != 4 {
		t.Errorf("LocalDate = %04d-%02d-%02d, want 2024-07-04", y, m, d)
	}
	if h, mn, s := got.Clock(); h != 0 || mn != 0 || s != 0 {
		t.Errorf("LocalDate not truncated to midnight: %v", got)
	}
}

func TestSunsetStrFutureAndPast(t *testing.T) {
	lat, lon := 40.7128, -74.0060

	morningUTC := time.Date(2024, 7, 4, 12, 0, 0, 0, time.UTC)
	got := SunsetStr(morningUTC, lat, lon)
	if !strings.HasPrefix(got, "Sunset on this day is in") {
		t.Fatalf("expected future-tense sunset, got %q", got)
	}

	nightUTC := time.Date(2024, 7, 5, 2, 0, 0, 0, time.UTC)
	got = SunsetStr(nightUTC, lat, lon)
	if !strings.HasPrefix(got, "Sunset on this day was") || !strings.HasSuffix(got, "ago") {
		t.Fatalf("expected past-tense sunset, got %q", got)
	}
}
