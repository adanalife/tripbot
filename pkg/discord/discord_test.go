package discord

import "testing"

func TestIsPlaceholderToken(t *testing.T) {
	tests := []struct {
		name string
		tok  string
		want bool
	}{
		{"real token", "not-a-real-token.aaaaaa.bbbbbb", false},
		{"empty", "", true},
		{"bare placeholder string", "placeholder — set via aws ssm put-parameter", true},
		{"json placeholder blob", `{"placeholder":"set via aws ssm put-parameter"}`, true},
		{"json object without the key", `{"token":"real"}`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isPlaceholderToken(tt.tok); got != tt.want {
				t.Errorf("isPlaceholderToken(%q) = %v, want %v", tt.tok, got, tt.want)
			}
		})
	}
}
