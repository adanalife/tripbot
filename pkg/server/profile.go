package server

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/adanalife/tripbot/pkg/events"
	"github.com/adanalife/tripbot/pkg/scoreboards"
	"github.com/adanalife/tripbot/pkg/users"
	"github.com/gorilla/mux"
	"gorm.io/gorm"
)

// findUser / sessionCount / monthlyMiles are the data seams the profile handler
// reads through, overridable in tests so the handler renders without a real DB.
// Each is an operator-triggered read (one click), not a hot path, so the extra
// monthly-score query is fine.
var (
	findUser      = users.Find
	sessionCount  = events.SessionCount
	earliestEvent = events.EarliestRealEventDate
	monthlyMiles  = func(ctx context.Context, u users.User) float32 {
		return u.GetScore(ctx, scoreboards.CurrentMilesScoreboard())
	}
)

// bestEffortFirstSeen picks the earliest non-zero timestamp among the user
// row's own dates and their earliest real event. The users row is authoritative
// when present (FirstSeen/DateCreated are stamped on insert now), but accounts
// created during the date_created bug window have zero-value dates there — for
// those, the earliest non-bug event row is the best surviving evidence of when
// we first saw them. Returns the zero time only when nothing real is available.
func bestEffortFirstSeen(times ...time.Time) time.Time {
	var best time.Time
	for _, t := range times {
		if t.IsZero() {
			continue
		}
		if best.IsZero() || t.Before(best) {
			best = t
		}
	}
	return best
}

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
	u, err := findUser(ctx, username)
	if err != nil {
		// not-found renders as Found=false; a real DB error does too, but
		// gets logged so it's visible as a failure rather than a ghost user.
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			slog.ErrorContext(ctx, "error finding user", "err", err, "username", username)
		}
		return prof
	}
	prof.Found = true
	prof.IsBot = u.IsBot
	prof.Miles = u.Miles
	prof.MonthlyMiles = monthlyMiles(ctx, u)
	prof.FirstSeen = bestEffortFirstSeen(u.FirstSeen, u.DateCreated, earliestEvent(ctx, u.Platform, username))
	prof.LastSeen = u.LastSeen
	prof.Sessions = sessionCount(ctx, username)
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
