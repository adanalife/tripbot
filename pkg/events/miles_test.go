package events

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/adanalife/tripbot/pkg/database/testdb"
	"github.com/google/uuid"
)

// TestMilesGained checks the window's edges against the real schema. The
// window is two hours, so the whole of it is 4.0 miles; explicit times
// throughout, since NOW() is frozen inside the test transaction.
func TestMilesGained(t *testing.T) {
	since := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
	until := since.Add(2 * time.Hour)
	type ev struct {
		event string
		at    time.Duration // offset from since
		sid   int           // session id; 0 for a correction
		extra float64
	}
	cases := []struct {
		name   string
		events []ev
		want   float64 // 0: absent from the result
	}{
		{"closed session inside the window", []ev{{"login", 10 * time.Minute, 1, 0}, {"logout", 40 * time.Minute, 1, 0}}, 1.0},
		{"session spanning the boundary counts its after part", []ev{{"login", -30 * time.Minute, 1, 0}, {"logout", 30 * time.Minute, 1, 0}}, 1.0},
		{"open session from before the boundary counts to until", []ev{{"login", -time.Hour, 1, 0}}, 4.0},
		{"open session inside the window counts from login", []ev{{"login", 90 * time.Minute, 1, 0}}, 1.0},
		{"superseded unpaired login pairs with nothing", []ev{{"login", 0, 1, 0}, {"login", 60 * time.Minute, 2, 0}, {"logout", 90 * time.Minute, 2, 0}}, 1.0},
		{"session over the cap is dropped", []ev{{"login", -23 * time.Hour, 1, 0}, {"logout", 90 * time.Minute, 1, 0}}, 0},
		{"nothing in the window", []ev{{"login", -2 * time.Hour, 1, 0}, {"logout", -time.Hour, 1, 0}}, 0},
		{"bonus and corrections inside the window", []ev{
			{"login", 0, 1, 0}, {"logout", 30 * time.Minute, 1, 2.5},
			{"correction", time.Hour, 0, -1}, {"correction", -time.Hour, 0, 50},
		}, 1.0 + 2.5 - 1},
	}
	db := testdb.New(t)
	var names []string
	for i, tc := range cases {
		user := fmt.Sprintf("gained_%d", i)
		names = append(names, user)
		for _, e := range tc.events {
			var sid any
			if e.sid != 0 {
				sid = uuid.NewSHA1(uuid.NameSpaceOID, fmt.Appendf(nil, "%s-%d", user, e.sid))
			}
			var extra any
			if e.extra != 0 {
				extra = e.extra
			}
			if err := db.Exec(`INSERT INTO events (platform, username, event, date_created, session_id, extra_miles_earned)
			                   VALUES ('twitch', ?, ?, ?, ?, ?)`, user, e.event, since.Add(e.at), sid, extra).Error; err != nil {
				t.Fatalf("insert: %v", err)
			}
		}
	}
	// The same user on another platform earns nothing on this one.
	if err := db.Exec(`INSERT INTO events (platform, username, event, date_created, extra_miles_earned)
	                   VALUES ('youtube', 'gained_0', 'correction', ?, 99)`, since.Add(time.Minute)).Error; err != nil {
		t.Fatalf("insert: %v", err)
	}

	got, err := MilesGained(context.Background(), "twitch", names, since, until)
	if err != nil {
		t.Fatalf("MilesGained: %v", err)
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			miles, ok := got[names[i]]
			switch {
			case tc.want == 0 && ok:
				t.Errorf("got %v, want absent", miles)
			case tc.want != 0 && math.Abs(miles-tc.want) > 0.001:
				t.Errorf("got %v, want %v", miles, tc.want)
			}
		})
	}
}
