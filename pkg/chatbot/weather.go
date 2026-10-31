package chatbot

import (
	"context"
	"time"

	"github.com/adanalife/tripbot/pkg/weather"
)

// Weather is the subset of weather lookup chatbot commands depend on (just
// historical conditions at a point, for !weather). Tests inject noopWeather;
// production uses pkg/weather's keyless Open-Meteo archive client, which the
// onscreens location feed shares.
type Weather interface {
	// Historical returns a short description of the weather at the given
	// coordinates around the given time (the time the clip was filmed).
	Historical(ctx context.Context, when time.Time, lat, lng float64) (string, error)
}

// realWeather is the production Weather.
var realWeather = weather.Archive{}
