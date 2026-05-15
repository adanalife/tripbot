package chatbot

import "testing"

func TestNormalizeCommandPrefix(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "leading inverted bang is rewritten to !",
			in:   "¡miles",
			want: "!miles",
		},
		{
			name: "inverted bang with params is rewritten to !",
			in:   "¡goto 42",
			want: "!goto 42",
		},
		{
			name: "regular ! prefix is untouched",
			in:   "!miles",
			want: "!miles",
		},
		{
			name: "bare-word (no prefix) is untouched",
			in:   "hello",
			want: "hello",
		},
		{
			name: "inverted bang not at the start is untouched",
			in:   "say ¡hola",
			want: "say ¡hola",
		},
		{
			name: "empty string is untouched",
			in:   "",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeCommandPrefix(tt.in)
			if got != tt.want {
				t.Errorf("normalizeCommandPrefix(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestNormalizeCommandPrefix_DispatchEquivalence asserts the same downstream
// "command" token (i.e. the first whitespace-separated word of the normalized
// message) is produced for `¡foo` and `!foo`. This is the property the
// dispatcher in runCommand() relies on for both prefixes to route to the same
// switch case.
func TestNormalizeCommandPrefix_DispatchEquivalence(t *testing.T) {
	cases := []string{"miles", "location", "leaderboard", "goto 42"}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			gotBang := normalizeCommandPrefix("!" + c)
			gotInverted := normalizeCommandPrefix("¡" + c)
			if gotBang != gotInverted {
				t.Errorf("dispatch divergence: !%s -> %q vs ¡%s -> %q",
					c, gotBang, c, gotInverted)
			}
		})
	}
}
