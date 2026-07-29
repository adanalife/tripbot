package weather

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// filmedAt is the moment the clip under test was filmed. The 18:00 hour is the
// interesting part: it's the index Historical must read out of the day's hourly
// samples, and it's far enough from both ends that an off-by-one or a fallback
// to sample 0 shows up as a wrong answer rather than a coincidence.
var filmedAt = time.Date(2018, 9, 14, 18, 30, 0, 0, time.UTC)

// serveArchive points archiveURL at an httptest server running h for the
// duration of the test. Rebinding a package var, so not safe for t.Parallel.
func serveArchive(t *testing.T, h http.HandlerFunc) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	orig := archiveURL
	archiveURL = srv.URL
	t.Cleanup(func() { archiveURL = orig })
}

// serveJSON is serveArchive for the common case: one fixed status and body.
func serveJSON(t *testing.T, status int, body string) {
	t.Helper()
	serveArchive(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	})
}

// hourlyJSON renders an archive response from per-hour temperatures and codes.
// Marshalled rather than hand-written so a field rename in the response struct
// surfaces as a decode miss instead of a typo hunt.
func hourlyJSON(t *testing.T, temps []float64, codes []int) string {
	t.Helper()
	b, err := json.Marshal(map[string]any{
		"hourly": map[string]any{
			"temperature_2m": temps,
			"weather_code":   codes,
		},
	})
	if err != nil {
		t.Fatalf("marshalling the fixture: %v", err)
	}
	return string(b)
}

// dayOfSamples returns 24 hourly samples whose temperature equals the hour, so
// an assertion on the temperature doubles as an assertion on the index picked.
func dayOfSamples(code int) ([]float64, []int) {
	temps := make([]float64, 24)
	codes := make([]int, 24)
	for i := range temps {
		temps[i] = float64(i)
		codes[i] = code
	}
	return temps, codes
}

// The sample index has to track the hour the clip was filmed — reading sample 0
// or the last sample would report a plausible temperature from the wrong part
// of the day, which is the failure mode nothing else would catch.
func TestHistorical_ReadsTheSampleForTheHourFilmed(t *testing.T) {
	temps, codes := dayOfSamples(2) // Partly cloudy all day
	codes[filmedAt.Hour()] = 61     // ...except the hour we want: Rain
	serveJSON(t, http.StatusOK, hourlyJSON(t, temps, codes))

	got, err := Archive{}.Historical(context.Background(), filmedAt, 39.5, -105.0)
	if err != nil {
		t.Fatalf("Historical: %v", err)
	}
	// temps[18] == 18, and the code at 18 is the Rain outlier.
	if want := "Rain, 18°F"; got != want {
		t.Errorf("Historical = %q, want %q", got, want)
	}
}

func TestHistorical_RoundsTheTemperature(t *testing.T) {
	temps, codes := dayOfSamples(0)
	temps[filmedAt.Hour()] = 70.6
	serveJSON(t, http.StatusOK, hourlyJSON(t, temps, codes))

	got, err := Archive{}.Historical(context.Background(), filmedAt, 39.5, -105.0)
	if err != nil {
		t.Fatalf("Historical: %v", err)
	}
	if want := "Clear sky, 71°F"; got != want {
		t.Errorf("Historical = %q, want %q", got, want)
	}
}

// A day the archive returned fewer than 24 samples for still has to answer,
// clamped to the last sample it did return.
func TestHistorical_ClampsToTheLastSampleOnAShortDay(t *testing.T) {
	serveJSON(t, http.StatusOK, hourlyJSON(t, []float64{10, 20, 30}, []int{0, 0, 71}))

	got, err := Archive{}.Historical(context.Background(), filmedAt, 39.5, -105.0)
	if err != nil {
		t.Fatalf("Historical: %v", err)
	}
	// Hour 18 is past the end, so the last sample answers: 30°F, code 71 (Snow).
	if want := "Snow, 30°F"; got != want {
		t.Errorf("Historical = %q, want %q", got, want)
	}
}

// weather_code can come back shorter than temperature_2m. The temperature is
// still worth reporting, so the code falls back to 0 rather than the whole
// lookup failing.
func TestHistorical_FallsBackToClearWhenTheCodeIsMissing(t *testing.T) {
	temps, _ := dayOfSamples(0)
	serveJSON(t, http.StatusOK, hourlyJSON(t, temps, []int{95, 95}))

	got, err := Archive{}.Historical(context.Background(), filmedAt, 39.5, -105.0)
	if err != nil {
		t.Fatalf("Historical: %v", err)
	}
	if want := "Clear sky, 18°F"; got != want {
		t.Errorf("Historical = %q, want %q", got, want)
	}
}

func TestHistorical_SurfacesFailures(t *testing.T) {
	cases := []struct {
		name    string
		status  int
		body    string
		wantErr string
	}{
		{"server error", http.StatusInternalServerError, `{}`, "status 500"},
		{"not found", http.StatusNotFound, `{}`, "status 404"},
		{"rate limited", http.StatusTooManyRequests, `{}`, "status 429"},
		{"malformed json", http.StatusOK, `{"hourly":`, "unexpected EOF"},
		{"not json at all", http.StatusOK, `<html>down for maintenance</html>`, "invalid character"},
		{"no hourly key", http.StatusOK, `{}`, "no data for 2018-09-14"},
		{"empty samples", http.StatusOK, `{"hourly":{"temperature_2m":[]}}`, "no data for 2018-09-14"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			serveJSON(t, tc.status, tc.body)

			got, err := Archive{}.Historical(context.Background(), filmedAt, 39.5, -105.0)
			if err == nil {
				t.Fatalf("Historical = %q, want an error", got)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %q, want it to mention %q", err, tc.wantErr)
			}
			if got != "" {
				t.Errorf("Historical returned %q alongside an error, want an empty string", got)
			}
		})
	}
}

// The query string is the contract with Open-Meteo: the wrong date or a
// swapped lat/lng would still decode fine and report confident nonsense.
func TestHistorical_RequestsTheFilmedDateAndCoordinates(t *testing.T) {
	var got url.Values
	temps, codes := dayOfSamples(0)
	serveArchive(t, func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Query()
		_, _ = io.WriteString(w, hourlyJSON(t, temps, codes))
	})

	if _, err := (Archive{}).Historical(context.Background(), filmedAt, 39.5, -105.0); err != nil {
		t.Fatalf("Historical: %v", err)
	}

	want := map[string]string{
		"latitude":         "39.5000",
		"longitude":        "-105.0000",
		"start_date":       "2018-09-14",
		"end_date":         "2018-09-14",
		"temperature_unit": "fahrenheit",
		"timezone":         "auto",
		"hourly":           "temperature_2m,weather_code",
	}
	for k, v := range want {
		if got.Get(k) != v {
			t.Errorf("query %s = %q, want %q", k, got.Get(k), v)
		}
	}
}

// A cancelled context has to come back as an error rather than a blank
// forecast, so a shutting-down chat handler doesn't render an empty overlay.
func TestHistorical_CancelledContext(t *testing.T) {
	temps, codes := dayOfSamples(0)
	serveJSON(t, http.StatusOK, hourlyJSON(t, temps, codes))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	got, err := Archive{}.Historical(ctx, filmedAt, 39.5, -105.0)
	if err == nil {
		t.Fatalf("Historical = %q, want an error for a cancelled context", got)
	}
	if got != "" {
		t.Errorf("Historical returned %q alongside an error, want an empty string", got)
	}
}
