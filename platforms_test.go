package tripbot

import "testing"

func TestPlatformsReadsTheSyncedFile(t *testing.T) {
	got := Platforms()
	if len(got) == 0 || got[0] != "twitch" {
		t.Fatalf("Platforms() = %v; want the synced list, twitch first", got)
	}
	got[0] = "mutated"
	if Platforms()[0] != "twitch" {
		t.Fatal("Platforms() handed out its own slice")
	}
}
