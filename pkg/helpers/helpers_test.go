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

func TestSplitOnRegex(t *testing.T) {
	tests := []struct {
		name      string
		text      string
		delimiter string
		want      []string
	}{
		{"simple comma split", "a,b,c", ",", []string{"a", "b", "c"}},
		{"empty input", "", ",", []string{""}},
		{"no matches", "abc", ",", []string{"abc"}},
		{"trailing delimiter yields empty tail", "a,b,", ",", []string{"a", "b", ""}},
		{"regex char class", "a1b2c3d", "[0-9]", []string{"a", "b", "c", "d"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SplitOnRegex(tt.text, tt.delimiter)
			if len(got) != len(tt.want) {
				t.Fatalf("got %v (len=%d), want %v (len=%d)", got, len(got), tt.want, len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("index %d: got %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
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

func TestBase64RoundTrip(t *testing.T) {
	cases := []string{"", "hello", "Hello, World!", "miles: 42.0\n@dana"}
	for _, in := range cases {
		t.Run(in, func(t *testing.T) {
			encoded := Base64Encode(in)
			decoded, err := Base64Decode(encoded)
			if err != nil {
				t.Fatalf("decode error: %v", err)
			}
			if decoded != in {
				t.Fatalf("round-trip mismatch: got %q, want %q", decoded, in)
			}
		})
	}
}

func TestBase64DecodeError(t *testing.T) {
	_, err := Base64Decode("!!!not-base64!!!")
	if err == nil {
		t.Fatal("expected decode error, got nil")
	}
}

func TestInvertMap(t *testing.T) {
	in := map[string]string{"a": "1", "b": "2", "c": "3"}
	got := InvertMap(in)
	want := map[string]string{"1": "a", "2": "b", "3": "c"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("key %q: got %q, want %q", k, got[k], v)
		}
	}
}

func TestInvertMapEmpty(t *testing.T) {
	got := InvertMap(map[string]string{})
	if len(got) != 0 {
		t.Fatalf("expected empty map, got %v", got)
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
