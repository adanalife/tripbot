package config

import "testing"

// UserIsAdmin is the only gate on the admin-only chat commands (!shutdown,
// !secretinfo) and on the rate-limit bypass every playback command shares, so
// it decides who can stop the bot and who can yank the playhead. Load
// lowercases ChannelName but a chat username arrives however the viewer's
// platform spells it — a display name on Twitch is mixed-case — so the
// comparison has to fold case. Swapping EqualFold for == locks Dana out of his
// own admin commands whenever the platform hands over "ADanaLife_".
func TestUserIsAdmin(t *testing.T) {
	cfg := TripbotConfig{ChannelName: "adanalife_"}

	for _, tc := range []struct {
		username string
		want     bool
	}{
		{"adanalife_", true},
		{"ADanaLife_", true},
		{"AdAnAlIfE_", true},
		{"adanalife", false}, // no trailing underscore: a different account
		{"adanalife__", false},
		{"someviewer", false},
		{"", false},
	} {
		if got := cfg.UserIsAdmin(tc.username); got != tc.want {
			t.Errorf("UserIsAdmin(%q) = %v, want %v", tc.username, got, tc.want)
		}
	}
}

// UserIsCompedSubscriber is checked before the real subscriber lookup, so it
// grants subscriber-only commands to people with no sub. Same case-folding
// requirement as UserIsAdmin, and the empty list is the default configuration
// — it must grant nothing rather than match everything.
func TestUserIsCompedSubscriber(t *testing.T) {
	cfg := TripbotConfig{CompedSubscribers: []string{"friendo", "OtherFriend"}}

	for _, tc := range []struct {
		username string
		want     bool
	}{
		{"friendo", true},
		{"FRIENDO", true},
		{"otherfriend", true},
		{"OtherFriend", true},
		{"friend", false},
		{"someviewer", false},
		{"", false},
	} {
		if got := cfg.UserIsCompedSubscriber(tc.username); got != tc.want {
			t.Errorf("UserIsCompedSubscriber(%q) = %v, want %v", tc.username, got, tc.want)
		}
	}

	var none TripbotConfig
	if none.UserIsCompedSubscriber("anyone") {
		t.Error("UserIsCompedSubscriber granted a comp with no allowlist configured")
	}
}

// The environment predicates fan out from one string, and a typo in any of
// them silently puts a production process on a development code path.
func TestEnvironmentPredicatesAreMutuallyExclusive(t *testing.T) {
	for _, tc := range []struct {
		env                        string
		prod, staging, development bool
	}{
		{"production", true, false, false},
		{"staging", false, true, false},
		{"development", false, false, true},
		{"testing", false, false, false},
		{"", false, false, false},
		{"Production", false, false, false}, // exact match: envconfig doesn't fold this one
	} {
		cfg := TripbotConfig{Environment: tc.env}
		if got := cfg.IsProduction(); got != tc.prod {
			t.Errorf("IsProduction() with Environment=%q = %v, want %v", tc.env, got, tc.prod)
		}
		if got := cfg.IsStaging(); got != tc.staging {
			t.Errorf("IsStaging() with Environment=%q = %v, want %v", tc.env, got, tc.staging)
		}
		if got := cfg.IsDevelopment(); got != tc.development {
			t.Errorf("IsDevelopment() with Environment=%q = %v, want %v", tc.env, got, tc.development)
		}
	}
}
