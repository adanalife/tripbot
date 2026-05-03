package twitch

import "testing"

func TestUserIsSubscriber(t *testing.T) {
	saved := subscribers
	defer func() { subscribers = saved }()

	subscribers = []string{"alice", "bob", "carol"}

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
			if got := UserIsSubscriber(tt.username); got != tt.want {
				t.Fatalf("UserIsSubscriber(%q) = %v, want %v", tt.username, got, tt.want)
			}
		})
	}
}

func TestUserIsSubscriberEmptyList(t *testing.T) {
	saved := subscribers
	defer func() { subscribers = saved }()

	subscribers = nil
	if UserIsSubscriber("anyone") {
		t.Fatal("expected false when subscribers is nil")
	}
}
