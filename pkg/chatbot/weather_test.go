package chatbot

import (
	"context"
	"testing"
	"time"
)

// noopWeather is the default Weather in newTestApp: every lookup returns an
// empty description and no error, so command tests don't make HTTP calls.
type noopWeather struct{}

func (noopWeather) Historical(_ context.Context, _ time.Time, _, _ float64) (string, error) {
	return "", nil
}

// recordingWeather captures the Historical call and returns a staged result,
// so tests can assert what !weather looked up and stage a description.
type recordingWeather struct {
	Result string
	Err    error

	Calls []string
}

func (r *recordingWeather) Historical(_ context.Context, when time.Time, lat, lng float64) (string, error) {
	r.Calls = append(r.Calls, when.Format("2006-01-02"))
	return r.Result, r.Err
}

func TestWeatherCodeText(t *testing.T) {
	cases := map[int]string{
		0:   "Clear sky",
		2:   "Partly cloudy",
		45:  "Foggy",
		63:  "Rain",
		75:  "Snow",
		95:  "Thunderstorm",
		999: "Unknown conditions",
	}
	for code, want := range cases {
		if got := weatherCodeText(code); got != want {
			t.Errorf("weatherCodeText(%d) = %q, want %q", code, got, want)
		}
	}
}
