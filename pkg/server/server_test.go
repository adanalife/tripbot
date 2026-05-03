package server

import (
	"testing"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
)

// isValidSecret is misnamed: it returns TRUE when the secret is *invalid*
// (empty or mismatched). These tests pin that existing inverted behavior so
// future renames don't silently flip auth.
func TestIsValidSecret(t *testing.T) {
	saved := c.Conf.TripbotHttpAuth
	defer func() { c.Conf.TripbotHttpAuth = saved }()
	c.Conf.TripbotHttpAuth = "secret-token"

	tests := []struct {
		name   string
		input  string
		wantInvalid bool
	}{
		{"empty string is treated as invalid", "", true},
		{"non-matching token is invalid", "wrong", true},
		{"matching token is valid", "secret-token", false},
		{"case mismatch is invalid", "Secret-Token", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isValidSecret(tt.input); got != tt.wantInvalid {
				t.Fatalf("isValidSecret(%q) = %v, want %v", tt.input, got, tt.wantInvalid)
			}
		})
	}
}
