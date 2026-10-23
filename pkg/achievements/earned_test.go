package achievements

import (
	"context"
	"testing"

	"github.com/adanalife/tripbot/pkg/database/testdb"
)

func TestEarnedBy(t *testing.T) {
	db := testdb.New(t)
	if err := db.Exec(`INSERT INTO achievements (platform, username, name, title, earned_at) VALUES
		('twitch', 'ada', 'landmark-old-faithful', 'Saw Old Faithful', '2026-10-01'),
		('twitch', 'ada', 'landmark-niagara-falls', 'Saw Niagara Falls', '2026-10-20'),
		('youtube', 'ada', 'landmark-devils-tower', 'Saw Devils Tower', '2026-10-21'),
		('twitch', 'bo', 'state-utah-1', 'First visit to Utah', '2026-10-22')`).Error; err != nil {
		t.Fatal(err)
	}
	got, err := EarnedBy(context.Background(), "twitch", "ada")
	if err != nil || len(got) != 2 || got[0].Title != "Saw Niagara Falls" || got[1].Title != "Saw Old Faithful" {
		t.Errorf("got %+v, %v", got, err)
	}
	if none, err := EarnedBy(context.Background(), "twitch", "nobody"); err != nil || none == nil || len(none) != 0 {
		t.Errorf("nobody: %#v, %v", none, err)
	}
}
