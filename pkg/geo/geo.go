// Package geo turns dashcam GPS coordinates into place names (city, state)
// via reverse geocoding. It wraps the kelvins/geocoder SDK behind a small
// Geocoder interface so command-time callers can inject a fake in tests
// instead of reaching for a package-level global and a live Google Maps call.
//
// The SDK exposes its API key only as a package-level global
// (geocoder.ApiKey), so New sets that global as a side effect. That global is
// an SDK constraint we can't remove here; what this package does remove is our
// own code reading it to make decisions — the disabled check now reads the
// Client's own apiKey field, and callers depend on the Geocoder interface
// rather than the SDK directly.
package geo

import (
	"errors"
	"fmt"

	"github.com/kelvins/geocoder"
)

// ErrDisabled is returned by City/State when no Google Maps API key is
// configured. Callers treat it as a soft-disable signal (skip the lookup,
// fall back to an empty result) rather than a real error — geocoding is
// optional and absent in local dev. Replaces helpers.ErrMapsDisabled.
var ErrDisabled = errors.New("geo: maps API disabled (no GOOGLE_MAPS_API_KEY set)")

// Geocoder is the reverse-geocoding surface command-time callers depend on.
// Production uses *Client; tests inject a fake.
type Geocoder interface {
	City(lat, lon float64) (string, error)
	State(lat, lon float64) (string, error)
}

// Client is the production Geocoder. Construct with New.
type Client struct {
	apiKey string
}

// New returns a Client configured with the given Google Maps API key, and
// sets the kelvins/geocoder package global so the SDK's reverse-geocode call
// authenticates. An empty key yields a Client whose City/State short-circuit
// with ErrDisabled (no HTTP, no Sentry noise).
func New(apiKey string) *Client {
	geocoder.ApiKey = apiKey
	return &Client{apiKey: apiKey}
}

// City returns "City, State" for the coordinates, or "Somewhere in <State>"
// when the SDK has no city for the point. Returns ErrDisabled when no key is
// configured.
func (c *Client) City(lat, lon float64) (string, error) {
	if c.apiKey == "" {
		return "", ErrDisabled
	}
	addresses, err := geocoder.GeocodingReverse(geocoder.Location{Latitude: lat, Longitude: lon})
	if err != nil {
		return "", err
	}
	// The SDK can return a nil error with zero addresses (e.g. ZERO_RESULTS
	// for coords outside any mapped area); indexing would panic.
	if len(addresses) == 0 {
		return "", fmt.Errorf("reverse geocode returned no addresses for %f,%f", lat, lon)
	}
	address := addresses[0]
	if address.City == "" {
		return fmt.Sprintf("Somewhere in %s", address.State), nil
	}
	return fmt.Sprintf("%s, %s", address.City, address.State), nil
}

// State returns the US state name for the coordinates. Returns ErrDisabled
// when no key is configured.
func (c *Client) State(lat, lon float64) (string, error) {
	if c.apiKey == "" {
		return "", ErrDisabled
	}
	addresses, err := geocoder.GeocodingReverse(geocoder.Location{Latitude: lat, Longitude: lon})
	if err != nil {
		return "", err
	}
	if len(addresses) == 0 {
		return "", fmt.Errorf("reverse geocode returned no addresses for %f,%f", lat, lon)
	}
	return addresses[0].State, nil
}

// defaultClient backs the package-level City/State free functions for callers
// that aren't constructed with a Geocoder dependency (pkg/video's import path).
// Disabled until SetDefault is called at startup.
var defaultClient Geocoder = &Client{}

// SetDefault installs the process-wide Geocoder used by the package-level
// City/State functions. Called once at startup (chatbot.Initialize, or a
// script's init) after config is loaded.
func SetDefault(g Geocoder) {
	defaultClient = g
}

// City reverse-geocodes via the process-wide default Geocoder.
func City(lat, lon float64) (string, error) { return defaultClient.City(lat, lon) }

// State reverse-geocodes via the process-wide default Geocoder.
func State(lat, lon float64) (string, error) { return defaultClient.State(lat, lon) }
