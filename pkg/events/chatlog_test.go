package events

import (
	"context"
	"testing"

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
