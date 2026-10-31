package chatbot

import "github.com/adanalife/tripbot/pkg/geo"

// Geocoder is the subset of reverse-geocoding that chatbot commands depend on:
// City for !location, State for grading a !guess at the playhead rather than
// against the clip's recorded state. Tests inject a fake; production uses
// realGeocoder, which delegates to the process-wide pkg/geo default configured
// in Initialize.
type Geocoder interface {
	City(lat, lon float64) (string, error)
	State(lat, lon float64) (string, error)
}

// realGeocoder routes to pkg/geo's package-level default Geocoder, installed
// via geo.SetDefault in Initialize once config is loaded.
type realGeocoder struct{}

func (realGeocoder) City(lat, lon float64) (string, error)  { return geo.City(lat, lon) }
func (realGeocoder) State(lat, lon float64) (string, error) { return geo.State(lat, lon) }
