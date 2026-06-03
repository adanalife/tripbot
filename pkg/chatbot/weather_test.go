package chatbot

import (
	"context"
	"strings"
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

func TestWeatherCmd_FlagOff_StaysSilent(t *testing.T) {
	app := newTestApp(newTestVideo("Wyoming", 43.0, -108.0, time.Date(2018, 3, 7, 15, 0, 0, 0, time.UTC)))
	chat := &recordingChat{}
	weather := &recordingWeather{Result: "Clear sky, 58°F"}
	app.Chat = chat
	app.Weather = weather
	// Flags defaults to noopFlags{} (every key false) — the fresh-deploy state.

	app.weatherCmd(context.Background(), newTestUser("viewer1"), nil)

	if len(chat.Says) != 0 {
		t.Errorf("flag off: expected no chat output, got %v", chat.Says)
	}
	if len(weather.Calls) != 0 {
		t.Errorf("flag off: expected no weather lookup, got %v", weather.Calls)
	}
}

func TestWeatherCmd_FlagOn_SaysConditionsWithoutDate(t *testing.T) {
	app := newTestApp(newTestVideo("Wyoming", 43.0, -108.0, time.Date(2018, 3, 7, 15, 0, 0, 0, time.UTC)))
	chat := &recordingChat{}
	app.Chat = chat
	app.Weather = &recordingWeather{Result: "Clear sky, 58°F"}
	app.Flags = &recordingFlags{Set: map[string]bool{weatherFlagKey: true}}

	app.weatherCmd(context.Background(), newTestUser("viewer1"), nil)

	if len(chat.Says) != 1 {
		t.Fatalf("flag on: expected exactly one chat message, got %d: %v", len(chat.Says), chat.Says)
	}
	if chat.Says[0] != "Clear sky, 58°F" {
		t.Errorf("unexpected message %q", chat.Says[0])
	}
	if strings.Contains(chat.Says[0], "2018") || strings.Contains(chat.Says[0], "Mar") {
		t.Errorf("message should not contain the filmed date, got %q", chat.Says[0])
	}
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
