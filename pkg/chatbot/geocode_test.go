package chatbot

// noopGeocoder is the default Geocoder in newTestApp: every lookup returns an
// empty string, no error. Commands that geocode but whose result the test
// doesn't assert on use this.
type noopGeocoder struct{}

func (noopGeocoder) City(_, _ float64) (string, error) { return "", nil }
