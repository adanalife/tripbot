package twitch

import "testing"

func TestUserIsSubscriber(t *testing.T) {
	cl := New()
	cl.subscribers = map[string]int{"alice": 1, "bob": 2, "carol": 3}

	tests := []struct {
		username string
		want     bool
	}{
		{"alice", true},
		{"bob", true},
		{"carol", true},
		{"dave", false},
		{"", false},
		{"Alice", false},
	}
	for _, tt := range tests {
		t.Run(tt.username, func(t *testing.T) {
			if got := cl.UserIsSubscriber(tt.username); got != tt.want {
				t.Fatalf("UserIsSubscriber(%q) = %v, want %v", tt.username, got, tt.want)
			}
		})
	}
}

func TestUserIsSubscriberEmptyList(t *testing.T) {
	cl := New() // subscribers nil
	if cl.UserIsSubscriber("anyone") {
		t.Fatal("expected false when subscribers is nil")
	}
}

func TestUserSubscriberTier(t *testing.T) {
	cl := New()
	// SetSubscribers lowercases logins and clamps missing/zero tiers to 1
	cl.SetSubscribers(map[string]int{"Alice": 3, "bob": 0})

	tests := []struct {
		username string
		want     int
	}{
		{"alice", 3},
		{"bob", 1},
		{"dave", 0},
	}
	for _, tt := range tests {
		t.Run(tt.username, func(t *testing.T) {
			if got := cl.UserSubscriberTier(tt.username); got != tt.want {
				t.Fatalf("UserSubscriberTier(%q) = %d, want %d", tt.username, got, tt.want)
			}
		})
	}
}
