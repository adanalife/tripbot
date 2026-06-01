package chatbot

import "fmt"

// noopGeocoder is the default Geocoder in newTestApp: every lookup returns an
// empty string, no error. Commands that geocode but whose result the test
// doesn't assert on use this.
type noopGeocoder struct{}

func (noopGeocoder) City(_, _ float64) (string, error) { return "", nil }

// recordingGeocoder captures City calls and returns a staged result, so tests
// can assert the command geocoded the expected coordinates.
type recordingGeocoder struct {
	City_ string // staged return value
	Err   error  // staged error
	Calls []string
}

func (r *recordingGeocoder) City(lat, lon float64) (string, error) {
	r.Calls = append(r.Calls, fmt.Sprintf("City(%v,%v)", lat, lon))
	return r.City_, r.Err
}
