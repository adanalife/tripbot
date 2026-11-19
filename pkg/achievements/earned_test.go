package achievements

import (
	"context"
	"testing"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/database/testdb"
	"github.com/adanalife/tripbot/pkg/video"
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
	if err != nil || len(got) != 2 || got[0].Name != "landmark-niagara-falls" || got[0].Title != "Saw Niagara Falls" || got[1].Title != "Saw Old Faithful" {
		t.Errorf("got %+v, %v", got, err)
	}
	if none, err := EarnedBy(context.Background(), "twitch", "nobody"); err != nil || none == nil || len(none) != 0 {
		t.Errorf("nobody: %#v, %v", none, err)
	}
}

// The counter pass awards every tier a viewer in chat has reached, once, and
// leaves the viewers who aren't in chat alone.
func TestHandleVideoChange_AwardsCounterTiers(t *testing.T) {
	db := testdb.New(t)
	if err := db.Exec(`INSERT INTO users (platform, username, miles) VALUES
		('twitch', 'ada', 150), ('twitch', 'bo', 9.9), ('twitch', 'away', 5000)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO events (platform, username, event, meta, date_created) VALUES
		('twitch', 'bo', 'guess_submitted', '{"correct":true}', now()),
		('twitch', 'bo', 'guess_submitted', '{"correct":false}', now()),
		('youtube', 'ada', 'guess_submitted', '{"correct":true}', now())`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO user_state_days (platform, username, state, day)
		SELECT 'twitch', 'ada', 'Utah', CURRENT_DATE - d FROM generate_series(1, 6) d`).Error; err != nil {
		t.Fatal(err)
	}
	conf := &c.TripbotConfig{Environment: "testing", Platform: "twitch"}
	// Flagged skips the landmark pass; today's Utah row is ada's 7th day.
	v := video.Video{ID: 1, State: "Utah", Flagged: true}

	first := HandleVideoChange(context.Background(), conf, v, []string{"ada", "bo"})
	again := HandleVideoChange(context.Background(), conf, v, []string{"ada", "bo"})
	if len(again) != 0 {
		t.Errorf("second pass re-announced: %v", again)
	}

	var got []string
	if err := db.Raw(`SELECT username || ':' || name FROM achievements ORDER BY 1`).Scan(&got).Error; err != nil {
		t.Fatal(err)
	}
	want := []string{"ada:days-7", "ada:miles-10", "ada:miles-100", "ada:state-utah-1", "bo:guesses-1", "bo:state-utah-1"}
	if len(got) != len(want) {
		t.Fatalf("awarded %v (announced %v), want %v", got, first, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("awarded %v, want %v", got, want)
			break
		}
	}
}

func TestStateDaysForAndStatesAired(t *testing.T) {
	db := testdb.New(t)
	if err := db.Exec(`INSERT INTO user_state_days (platform, username, state, day) VALUES
		('twitch', 'ada', 'Utah', '2026-10-01'), ('twitch', 'ada', 'Utah', '2026-10-02'),
		('twitch', 'ada', 'Ohio', '2026-10-02'), ('youtube', 'ada', 'Iowa', '2026-10-02')`).Error; err != nil {
		t.Fatal(err)
	}
	got, err := StateDaysFor(context.Background(), "twitch", "ada")
	if err != nil || len(got) != 2 || got[0] != (StateDays{"Utah", 2}) || got[1] != (StateDays{"Ohio", 1}) {
		t.Errorf("got %+v, %v", got, err)
	}
	if none, err := StateDaysFor(context.Background(), "twitch", "nobody"); err != nil || none == nil || len(none) != 0 {
		t.Errorf("nobody: %#v, %v", none, err)
	}
	if err := db.Exec(`INSERT INTO video_plays (platform, state) VALUES
		('twitch', 'Utah'), ('twitch', 'Utah'), ('twitch', 'Ohio'), ('twitch', ''), ('youtube', 'Iowa')`).Error; err != nil {
		t.Fatal(err)
	}
	if n, err := StatesAired(context.Background(), "twitch"); err != nil || n != 2 {
		t.Errorf("StatesAired = %d, %v, want 2", n, err)
	}
}
