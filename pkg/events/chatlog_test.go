package events

import (
	"context"
	"testing"
	"time"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/database/testdb"
)

// A line lands with the writing platform and its airing stamp; a line with no
// clip writes NULL rather than 0.
func TestRecordChat(t *testing.T) {
	db := testdb.New(t)
	cfg := &c.TripbotConfig{Platform: "twitch"}
	secs := 42.5
	if err := RecordChat(context.Background(), cfg, ChatLine{Username: "alice", Text: "wow", VideoID: 7, TsSec: &secs}); err != nil {
		t.Fatal(err)
	}
	if err := RecordChat(context.Background(), cfg, ChatLine{Username: "tripbot", Text: "hi"}); err != nil {
		t.Fatal(err)
	}
	var rows []chatRow
	if err := db.Order("id").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	if rows[0].Platform != "twitch" || rows[0].VideoID == nil || *rows[0].VideoID != 7 || rows[0].VideoTsSec == nil || *rows[0].VideoTsSec != 42.5 {
		t.Errorf("stamped row = %+v", rows[0])
	}
	if rows[1].VideoID != nil || rows[1].VideoTsSec != nil || rows[1].EmittedAt.IsZero() {
		t.Errorf("bare row = %+v", rows[1])
	}
	if err := RecordChat(context.Background(), &c.TripbotConfig{Platform: "twitch", ReadOnly: true}, ChatLine{Username: "x", Text: "y"}); err == nil {
		t.Error("read-only instance should refuse to write")
	}
}

// A message id strikes that one line; a user-wide removal strikes only that
// user's recent lines on this platform; a struck line keeps its first stamp.
func TestStrikeChat(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	cfg := &c.TripbotConfig{Platform: "twitch"}
	for _, l := range []ChatLine{
		{Username: "alice", UserID: "1", MessageID: "m1", Text: "one"},
		{Username: "alice", UserID: "1", MessageID: "m2", Text: "two"},
		{Username: "bob", UserID: "2", MessageID: "m3", Text: "three"},
	} {
		if err := RecordChat(ctx, cfg, l); err != nil {
			t.Fatal(err)
		}
	}
	// An old line of alice's, outside the user-wide window, and one of hers
	// on another platform.
	if err := db.Exec(`INSERT INTO chat_messages (platform, username, user_id, message_id, text, emitted_at)
		VALUES ('twitch', 'alice', '1', 'm0', 'old', now() - interval '2 hours'),
		       ('youtube', 'alice', '1', 'y1', 'elsewhere', now())`).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	struck := func() map[string]bool {
		var rows []chatRow
		if err := db.Find(&rows).Error; err != nil {
			t.Fatal(err)
		}
		m := map[string]bool{}
		for _, r := range rows {
			m[r.MessageID] = r.DeletedAt != nil
		}
		return m
	}

	if n, err := StrikeChat(ctx, cfg, "1", "m2", now); err != nil || n != 1 {
		t.Fatalf("one-message strike: n=%d err=%v", n, err)
	}
	if got := struck(); !got["m2"] || got["m1"] || got["m3"] {
		t.Fatalf("after one-message strike: %v", got)
	}

	if n, err := StrikeChat(ctx, cfg, "1", "", now); err != nil || n != 1 {
		t.Fatalf("user-wide strike: n=%d err=%v (want only m1: m2 already struck, m0 too old, y1 another platform)", n, err)
	}
	if got := struck(); !got["m1"] || got["m0"] || got["y1"] || got["m3"] {
		t.Fatalf("after user-wide strike: %v", got)
	}

	if n, err := StrikeChat(ctx, cfg, "", "", now); err != nil || n != 0 {
		t.Fatalf("idless strike: n=%d err=%v, want a no-op", n, err)
	}
	if _, err := StrikeChat(ctx, &c.TripbotConfig{Platform: "twitch", ReadOnly: true}, "2", "", now); err == nil {
		t.Error("read-only instance should refuse to write")
	}
}
