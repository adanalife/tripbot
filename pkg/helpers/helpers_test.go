package helpers

import (
	"strings"
	"testing"
	"time"
)

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
	// date_filmed holds the dashcam's wall clock labelled UTC: noon on the
	// camera in New York is 12:00 EDT, which is 16:00Z.
	filmed := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	got := ActualDate(filmed, 40.7128, -74.0060)
	if got.Location().String() != "America/New_York" {
		t.Fatalf("expected America/New_York timezone, got %s", got.Location().String())
	}
	if h, m, _ := got.Clock(); h != 12 || m != 0 {
		t.Errorf("clock = %02d:%02d, want the camera's 12:00", h, m)
	}
	if want := time.Date(2024, 6, 15, 16, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Errorf("instant = %v, want %v", got.UTC(), want)
	}
}

func TestActualDateOutsideAnyZone(t *testing.T) {
	filmed := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	if got := ActualDate(filmed, 0, -140); !got.Equal(filmed) {
		t.Errorf("open ocean: got %v, want the stored clock as UTC", got)
	}
}

func TestIsDaytime(t *testing.T) {
	lat, lon := 40.7128, -74.0060 // New York

	// The camera's clocks, local to New York.
	if !IsDaytime(time.Date(2024, 7, 4, 8, 0, 0, 0, time.UTC), lat, lon) {
		t.Error("expected 08:00 to be daytime")
	}
	if IsDaytime(time.Date(2024, 7, 4, 22, 0, 0, 0, time.UTC), lat, lon) {
		t.Error("expected 22:00 to be nighttime")
	}
	// 19:00 in July is before New York's ~20:30 sunset. Read as a UTC instant
	// it would be 15:00 local — daytime either way — so check the evening the
	// old reading got wrong: 21:30 is dark locally but was 17:30 as UTC.
	if IsDaytime(time.Date(2024, 7, 4, 21, 30, 0, 0, time.UTC), lat, lon) {
		t.Error("expected 21:30 to be nighttime (after sunset)")
	}
	// And the morning: 05:00 is before the ~05:30 sunrise, but was 01:00 the
	// previous night as UTC.
	if IsDaytime(time.Date(2024, 7, 4, 5, 0, 0, 0, time.UTC), lat, lon) {
		t.Error("expected 05:00 to be before sunrise")
	}
	if !IsDaytime(time.Date(2024, 7, 4, 19, 0, 0, 0, time.UTC), lat, lon) {
		t.Error("expected 19:00 to be daytime")
	}
}

func TestLocalDate(t *testing.T) {
	lat, lon := 40.7128, -74.0060 // New York

	// 22:00 on the camera on July 4 is July 4 locally, even though the real
	// instant (02:00Z) falls on the 5th.
	got := LocalDate(time.Date(2024, 7, 4, 22, 0, 0, 0, time.UTC), lat, lon)
	if y, m, d := got.Date(); y != 2024 || m != time.July || d != 4 {
		t.Errorf("LocalDate = %04d-%02d-%02d, want 2024-07-04", y, m, d)
	}
	if h, mn, s := got.Clock(); h != 0 || mn != 0 || s != 0 {
		t.Errorf("LocalDate not truncated to midnight: %v", got)
	}
}

func TestSunsetStrFutureAndPast(t *testing.T) {
	lat, lon := 40.7128, -74.0060

	got := SunsetStr(time.Date(2024, 7, 4, 8, 0, 0, 0, time.UTC), lat, lon)
	if !strings.HasPrefix(got, "Sunset on this day is in") {
		t.Fatalf("expected future-tense sunset, got %q", got)
	}

	got = SunsetStr(time.Date(2024, 7, 4, 22, 0, 0, 0, time.UTC), lat, lon)
	if !strings.HasPrefix(got, "Sunset on this day was") || !strings.HasSuffix(got, "ago") {
		t.Fatalf("expected past-tense sunset, got %q", got)
	}
}

func TestSunsetAtIsLocalEvening(t *testing.T) {
	// New York's sunset on July 4 is ~20:31 EDT; formatted, it must read as an
	// evening clock time in the filming zone, not a UTC one.
	got := SunsetAt(time.Date(2024, 7, 4, 12, 0, 0, 0, time.UTC), 40.7128, -74.0060)
	if got.Location().String() != "America/New_York" {
		t.Fatalf("zone = %s, want America/New_York", got.Location())
	}
	if h := got.Hour(); h != 20 {
		t.Errorf("sunset hour = %d, want 20 (local)", h)
	}
}

func TestTimeAgoSince(t *testing.T) {
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		then time.Time
		want string
	}{
		{"years and months", time.Date(2023, 7, 3, 12, 0, 0, 0, time.UTC), "3 years 2 months ago"},
		{"whole years only", time.Date(2024, 9, 3, 12, 0, 0, 0, time.UTC), "2 years ago"},
		{"one year singular", time.Date(2025, 9, 3, 12, 0, 0, 0, time.UTC), "1 year ago"},
		{"months only", time.Date(2026, 4, 3, 12, 0, 0, 0, time.UTC), "5 months ago"},
		{"one month singular", time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC), "1 month ago"},
		{"day-of-month not yet reached", time.Date(2025, 9, 30, 12, 0, 0, 0, time.UTC), "11 months ago"},
		{"under a month falls back to durafmt", time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC), "2 weeks ago"},
		{"zero timestamp", time.Time{}, ""},
		{"future timestamp", now.Add(time.Hour), ""},
		{"now", now, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := timeAgoSince(tt.then, now); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}
