package video

import "testing"

// Place is what chat hears, so its three tiers have to read correctly — and an
// unresolved moment must render empty, since that's the caller's only signal to
// fall back to the live geocoder rather than saying something wrong.
func TestMomentPlace(t *testing.T) {
	tests := []struct {
		name string
		m    Moment
		want string
	}{
		{"inside a place", Moment{State: "California", City: "Bishop"}, "Bishop, California"},
		{"near a place", Moment{State: "Wyoming", City: "Mammoth", CityM: 4200}, "near Mammoth, Wyoming"},
		{"at the near limit", Moment{State: "Wyoming", City: "Cody", CityM: nearPlaceLimit}, "near Cody, Wyoming"},
		{"past the near limit", Moment{State: "Wyoming", City: "Cody", CityM: nearPlaceLimit + 1}, "Somewhere in Wyoming"},
		{"state only", Moment{State: "Nevada"}, "Somewhere in Nevada"},
		{"ungeocoded", Moment{Lat: 39.5, Lng: -105}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.m.Place(); got != tt.want {
				t.Errorf("Place() = %q, want %q", got, tt.want)
			}
		})
	}
}
