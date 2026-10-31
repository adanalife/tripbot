package rotatorstore

import (
	"testing"

	"github.com/adanalife/tripbot/pkg/database/testdb"
	rot "github.com/adanalife/tripbot/pkg/rotator"
)

// newStore hands back a Store on a transaction-scoped connection; testdb skips
// the calling test when postgres is unreachable.
func newStore(t *testing.T) *Store {
	t.Helper()
	return New(testdb.New(t))
}

// TestGetOrDefaultFallsBackToShippedCopy is the state every platform starts in:
// nothing stored, so the console prefills from the copy compiled into the binary
// and reports that it isn't saved copy.
func TestGetOrDefaultFallsBackToShippedCopy(t *testing.T) {
	s := newStore(t)

	cfg, stored, err := s.GetOrDefault(t.Context(), rot.PlatformYouTube)
	if err != nil {
		t.Fatalf("GetOrDefault: %v", err)
	}
	if stored {
		t.Error("expected stored=false for a platform that was never edited")
	}
	// The prefill is platform-filtered, so YouTube must not carry Twitch-only copy.
	for _, m := range cfg.Left.Messages {
		if len(m.Platforms) != 0 {
			t.Errorf("prefill kept platform scoping on %q", m.Text)
		}
	}
	if len(cfg.Left.Messages) == 0 {
		t.Error("expected the YouTube prefill to carry some left-corner copy")
	}
}

func TestPutThenGetRoundTrips(t *testing.T) {
	s := newStore(t)
	want := rot.Config{
		Left: rot.Corner{
			Messages:      []rot.Message{{Text: "first", Weight: 3}, {Text: "second"}},
			PromoMessages: []rot.Message{{Text: "promo"}},
		},
		Right:       rot.Corner{Messages: []rot.Message{{Text: "right"}}},
		RareMessage: "rare",
	}

	if err := s.Put(t.Context(), rot.PlatformTwitch, want); err != nil {
		t.Fatalf("Put: %v", err)
	}
	got, stored, err := s.GetOrDefault(t.Context(), rot.PlatformTwitch)
	if err != nil {
		t.Fatalf("GetOrDefault: %v", err)
	}
	if !stored {
		t.Error("expected stored=true after a Put")
	}
	if len(got.Left.Messages) != 2 {
		t.Fatalf("left messages = %+v, want 2", got.Left.Messages)
	}
	// Order is the editor's ordering and has to survive the round trip.
	if got.Left.Messages[0].Text != "first" || got.Left.Messages[1].Text != "second" {
		t.Errorf("message order changed: %+v", got.Left.Messages)
	}
	if got.Left.Messages[0].Weight != 3 {
		t.Errorf("weight = %d, want 3", got.Left.Messages[0].Weight)
	}
	if got.RareMessage != "rare" {
		t.Errorf("rare = %q, want rare", got.RareMessage)
	}
}

// A second save for the same platform overwrites rather than erroring or
// accumulating rows — the console saves the whole document every time.
func TestPutUpserts(t *testing.T) {
	s := newStore(t)
	ctx := t.Context()

	if err := s.Put(ctx, rot.PlatformTikTok, rot.Config{
		Left: rot.Corner{Messages: []rot.Message{{Text: "before"}}},
	}); err != nil {
		t.Fatalf("first Put: %v", err)
	}
	if err := s.Put(ctx, rot.PlatformTikTok, rot.Config{
		Left: rot.Corner{Messages: []rot.Message{{Text: "after"}}},
	}); err != nil {
		t.Fatalf("second Put: %v", err)
	}

	got, _, err := s.GetOrDefault(ctx, rot.PlatformTikTok)
	if err != nil {
		t.Fatalf("GetOrDefault: %v", err)
	}
	if len(got.Left.Messages) != 1 || got.Left.Messages[0].Text != "after" {
		t.Errorf("left messages = %+v, want just the second save", got.Left.Messages)
	}
}

// Storing an empty config is a real edit ("show nothing in this corner"), and
// must read back as stored rather than falling through to the defaults.
func TestPutEmptyConfigIsStoredNotDefaulted(t *testing.T) {
	s := newStore(t)
	ctx := t.Context()

	if err := s.Put(ctx, rot.PlatformInstagram, rot.Config{}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	got, stored, err := s.GetOrDefault(ctx, rot.PlatformInstagram)
	if err != nil {
		t.Fatalf("GetOrDefault: %v", err)
	}
	if !stored {
		t.Fatal("an explicitly empty config should read back as stored")
	}
	if len(got.Left.Messages) != 0 || len(got.Right.Messages) != 0 {
		t.Errorf("expected empty pools, got %+v", got)
	}
}

func TestDeleteRevertsToDefaults(t *testing.T) {
	s := newStore(t)
	ctx := t.Context()

	if err := s.Put(ctx, rot.PlatformTwitch, rot.Config{
		Left: rot.Corner{Messages: []rot.Message{{Text: "edited"}}},
	}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if err := s.Delete(ctx, rot.PlatformTwitch); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, stored, err := s.GetOrDefault(ctx, rot.PlatformTwitch)
	if err != nil {
		t.Fatalf("GetOrDefault: %v", err)
	}
	if stored {
		t.Error("expected stored=false after Delete")
	}
}

// Deleting copy that was never stored reaches the same end state, so it isn't
// an error — the console's "reset to defaults" shouldn't care.
func TestDeleteUnknownPlatformIsNotAnError(t *testing.T) {
	if err := newStore(t).Delete(t.Context(), rot.PlatformFacebook); err != nil {
		t.Errorf("Delete on an unedited platform: %v", err)
	}
}

// Platforms backs tripbot's startup republish, which is what refills a wiped
// JetStream cache from the record of truth.
func TestPlatformsListsStoredOnly(t *testing.T) {
	s := newStore(t)
	ctx := t.Context()

	if err := s.Put(ctx, rot.PlatformYouTube, rot.Config{}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	got, err := s.Platforms(ctx)
	if err != nil {
		t.Fatalf("Platforms: %v", err)
	}
	if len(got) != 1 || got[0] != rot.PlatformYouTube {
		t.Errorf("Platforms = %v, want just youtube", got)
	}
}
