// Package weather answers "what was the weather here when this was filmed" for
// a dashcam clip, against the Open-Meteo historical reanalysis archive.
//
// Two callers share it: the chatbot's !weather command and the location feed
// that pushes live clip data to the onscreens rotators. It's stdlib-only and
// side-effect free (no config, no DB, no init) so either can import it.
package weather

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// archiveURL is the Open-Meteo historical reanalysis endpoint. It's free and
// keyless, and covers back to 1940 — so it can answer for the 2018 dashcam
// corpus. A var, not a const, so tests can point it at an httptest server;
// nothing outside this package can reach it.
var archiveURL = "https://archive-api.open-meteo.com/v1/archive"

// httpTimeout bounds the archive fetch so a hung Open-Meteo response can't
// stall a chat handler or a background tick.
const httpTimeout = 5 * time.Second

// Archive queries the Open-Meteo archive over HTTP. The zero value is ready to
// use; there's nothing to configure.
type Archive struct{}

// Historical returns a short description of the weather at the given
// coordinates around the given time, e.g. "Partly cloudy, 71°F".
func (Archive) Historical(ctx context.Context, when time.Time, lat, lng float64) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, httpTimeout)
	defer cancel()

	date := when.Format("2006-01-02")
	u := fmt.Sprintf(
		"%s?latitude=%.4f&longitude=%.4f&start_date=%s&end_date=%s&hourly=temperature_2m,weather_code&temperature_unit=fahrenheit&timezone=auto",
		archiveURL, lat, lng, date, date,
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("open-meteo archive: status %d", resp.StatusCode)
	}

	var body struct {
		Hourly struct {
			Temperature []float64 `json:"temperature_2m"`
			WeatherCode []int     `json:"weather_code"`
		} `json:"hourly"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", err
	}
	if len(body.Hourly.Temperature) == 0 {
		return "", fmt.Errorf("open-meteo archive: no data for %s", date)
	}

	// The archive returns 24 hourly samples for the day in local time
	// (timezone=auto), so the sample index lines up with the hour the clip
	// was filmed — as close to the moment as the reanalysis grid allows.
	idx := when.Hour()
	if idx >= len(body.Hourly.Temperature) {
		idx = len(body.Hourly.Temperature) - 1
	}
	temp := body.Hourly.Temperature[idx]
	code := 0
	if idx < len(body.Hourly.WeatherCode) {
		code = body.Hourly.WeatherCode[idx]
	}
	return fmt.Sprintf("%s, %.0f°F", codeText(code), temp), nil
}

// codeText maps a WMO weather-interpretation code (the WW codes Open-Meteo
// returns in weather_code) to a short human phrase.
// See https://open-meteo.com/en/docs.
func codeText(code int) string {
	switch code {
	case 0:
		return "Clear sky"
	case 1:
		return "Mainly clear"
	case 2:
		return "Partly cloudy"
	case 3:
		return "Overcast"
	case 45, 48:
		return "Foggy"
	case 51, 53, 55:
		return "Drizzle"
	case 56, 57:
		return "Freezing drizzle"
	case 61, 63, 65:
		return "Rain"
	case 66, 67:
		return "Freezing rain"
	case 71, 73, 75:
		return "Snow"
	case 77:
		return "Snow grains"
	case 80, 81, 82:
		return "Rain showers"
	case 85, 86:
		return "Snow showers"
	case 95:
		return "Thunderstorm"
	case 96, 99:
		return "Thunderstorm with hail"
	default:
		return "Unknown conditions"
	}
}
