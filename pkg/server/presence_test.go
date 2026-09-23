package server

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/adanalife/tripbot/pkg/database/testdb"
)

func TestPresenceInsightsHandler(t *testing.T) {
	db := testdb.New(t)
	seedUser(t, db, "pr_alice", false)
	seedUser(t, db, "pr_farm", false)
	seedUser(t, db, "pr_bot", true)
	if err := db.Exec(`UPDATE users SET exclude_from_leaderboard = true WHERE username = 'pr_alice'`).Error; err != nil {
		t.Fatalf("flag pr_alice: %v", err)
	}
	now := time.Now()

	seedSession(t, db, "twitch", "pr_alice", now.Add(-48*time.Hour), 120)
	seedSession(t, db, "twitch", "pr_alice", now.Add(-24*time.Hour), 180)
	// Began a day before the window and ended 12h ago: 156h inside it, 180h long.
	seedSession(t, db, "twitch", "pr_farm", now.Add(-8*24*time.Hour), 180*60)
	seedSession(t, db, "twitch", "pr_bot", now.Add(-30*time.Hour), 20*60)
	// Left out: an open login and a session wholly before the window.
	seedSession(t, db, "twitch", "pr_alice", now.Add(-time.Hour), -1)
	seedSession(t, db, "twitch", "pr_alice", now.Add(-10*24*time.Hour), 60)

	rec := insightsGET(t, "/api/insights/presence", presenceInsightsHandler)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var got presenceResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	want := []presenceRow{
		{Platform: "twitch", Username: "pr_farm", PresenceHours: 156, Sessions: 1, LongestSessionHours: 180},
		{Platform: "twitch", Username: "pr_bot", PresenceHours: 20, Sessions: 1, LongestSessionHours: 20, IsBot: true},
		{Platform: "twitch", Username: "pr_alice", PresenceHours: 5, Sessions: 2, LongestSessionHours: 3, ExcludeFromLeaderboard: true},
	}
	if got.Days != 7 || len(got.Accounts) != len(want) {
		t.Fatalf("got %+v, want days=7 and %d accounts", got, len(want))
	}
	for i := range want {
		if got.Accounts[i] != want[i] {
			t.Errorf("row %d = %+v, want %+v", i, got.Accounts[i], want[i])
		}
	}

	rec = insightsGET(t, "/api/insights/presence?limit=1", presenceInsightsHandler)
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Accounts) != 1 || got.Accounts[0].Username != "pr_farm" {
		t.Errorf("limit=1 returned %+v, want only pr_farm", got.Accounts)
	}
}
