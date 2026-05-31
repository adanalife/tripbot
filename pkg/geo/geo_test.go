package geo

import (
	"errors"
	"testing"
)

// A zero-key Client must short-circuit with ErrDisabled and never touch the
// network — this is the steady state in local dev and any env without a Maps
// key, and the whole point of the sentinel.
func TestDisabledClientShortCircuits(t *testing.T) {
	c := New("")

	if _, err := c.City(40.7, -111.8); !errors.Is(err, ErrDisabled) {
		t.Errorf("City with empty key: got %v, want ErrDisabled", err)
	}
	if _, err := c.State(40.7, -111.8); !errors.Is(err, ErrDisabled) {
		t.Errorf("State with empty key: got %v, want ErrDisabled", err)
	}
}

// The package-level default starts disabled, so City/State are safe to call
// before SetDefault runs (e.g. a code path reached before startup wiring).
func TestPackageDefaultDisabledByDefault(t *testing.T) {
	orig := defaultClient
	t.Cleanup(func() { defaultClient = orig })
	defaultClient = &Client{}

	if _, err := State(40.7, -111.8); !errors.Is(err, ErrDisabled) {
		t.Errorf("package State before SetDefault: got %v, want ErrDisabled", err)
	}
}

func TestSetDefaultInstallsGeocoder(t *testing.T) {
	orig := defaultClient
	t.Cleanup(func() { defaultClient = orig })

	SetDefault(stubGeocoder{city: "Provo, Utah", state: "Utah"})

	got, err := City(40.2, -111.6)
	if err != nil {
		t.Fatalf("City: unexpected error %v", err)
	}
	if got != "Provo, Utah" {
		t.Errorf("City: got %q, want %q", got, "Provo, Utah")
	}
}

type stubGeocoder struct {
	city, state string
}

func (s stubGeocoder) City(_, _ float64) (string, error)  { return s.city, nil }
func (s stubGeocoder) State(_, _ float64) (string, error) { return s.state, nil }
