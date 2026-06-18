package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/adanalife/tripbot/pkg/events"
	"github.com/adanalife/tripbot/pkg/scoreboards"
	"github.com/adanalife/tripbot/pkg/users"
	"github.com/gorilla/mux"
)

// findUser / sessionCount / monthlyMiles are the data seams the profile handler
// reads through, overridable in tests so the handler renders without a real DB.
// Each is an operator-triggered read (one click), not a hot path, so the extra
// monthly-score query is fine.
var (
	findUser     = users.Find
	sessionCount = events.SessionCount
	monthlyMiles = func(ctx context.Context, u users.User) float32 {
		return u.GetScore(ctx, scoreboards.CurrentMilesScoreboard())
	}
)

// userProfile is the chat-console popover payload — a small at-a-glance view of
// a chatter, events-derived per the tripbot-events-table-design ADR. The JSON
// tags are the wire format the standalone tripbot-console reads via
// GET /api/user/{username} (it has no DB access of its own and proxies here).
type userProfile struct {
	Username     string    `json:"username"`
	Found        bool      `json:"found"`
	IsBot        bool      `json:"is_bot"`
	Miles        float32   `json:"miles"`         // lifetime
	MonthlyMiles float32   `json:"monthly_miles"` // current month
	Sessions     int64     `json:"sessions"`
	FirstSeen    time.Time `json:"first_seen"`
	LastSeen     time.Time `json:"last_seen"`
}

// gatherUserProfile reads a chatter's at-a-glance stats through the DB seams.
// Operator-triggered (one click), not a hot path, so the extra monthly-score
// query is fine. An empty username or no matching row returns Found=false.
func gatherUserProfile(ctx context.Context, username string) userProfile {
	username = strings.ToLower(strings.TrimSpace(username))
	prof := userProfile{Username: username}
	if username == "" {
		return prof
	}
	if u := findUser(ctx, username); u.ID != 0 {
		prof.Found = true
		prof.IsBot = u.IsBot
		prof.Miles = u.Miles
		prof.MonthlyMiles = monthlyMiles(ctx, u)
		prof.FirstSeen = u.DateCreated
		prof.LastSeen = u.LastSeen
		prof.Sessions = sessionCount(ctx, username)
	}
	return prof
}

// userProfileAPIHandler serves GET /api/user/{username}: a chatter's
// at-a-glance stats as JSON, for the standalone tripbot-console to render its
// own popover (the console holds no DB access — it proxies here).
func userProfileAPIHandler(w http.ResponseWriter, r *http.Request) {
	prof := gatherUserProfile(r.Context(), mux.Vars(r)["username"])
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(prof); err != nil {
		slog.ErrorContext(r.Context(), "couldn't encode user profile", "err", err)
	}
}
