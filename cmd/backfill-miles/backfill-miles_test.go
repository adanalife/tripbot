package main

import (
	"math"
	"testing"
	"time"

	"github.com/adanalife/tripbot/pkg/database/testdb"
	"github.com/google/uuid"
)

// A login whose logout was never written pairs with nothing, so the sessions
// after it keep their own lengths instead of each borrowing the next logout.
func TestComputeMiles_OrphanedLoginDoesNotShiftLaterSessions(t *testing.T) {
	db := testdb.New(t)
	if err := db.Exec(`INSERT INTO users (platform, username, miles, is_bot) VALUES ('twitch', 'crashed', 0, false)`).Error; err != nil {
		t.Fatalf("insert user: %v", err)
	}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	add := func(event string, id uuid.UUID, at time.Time) {
		t.Helper()
		if err := db.Exec(`INSERT INTO events (platform, username, event, session_id, date_created)
		                   VALUES ('twitch', 'crashed', ?, ?, ?)`, event, id, at).Error; err != nil {
			t.Fatalf("insert %s: %v", event, err)
		}
	}
	add("login", uuid.New(), base)
	for _, start := range []time.Duration{time.Hour, 2 * time.Hour} {
		id := uuid.New()
		add("login", id, base.Add(start))
		add("logout", id, base.Add(start+30*time.Minute))
	}

	var got []struct {
		Username string
		Computed float64
	}
	if err := db.Raw(computeMilesSQL).Scan(&got).Error; err != nil {
		t.Fatalf("computeMilesSQL: %v", err)
	}
	// two 30-minute sessions at 0.1mi/3min
	if len(got) != 1 || math.Abs(got[0].Computed-2.0) > 0.01 {
		t.Errorf("computed = %+v, want crashed at 2.0 miles", got)
	}
}
