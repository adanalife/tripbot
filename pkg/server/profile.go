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
	findUserByID  = users.FindByPlatformUserID
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
// a chatter, derived from the append-only events table. The JSON
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

// resolveUser finds the chatter's row, preferring the platform's user id over
// the display name: the id survives a rename, where the name it was requested
// under may since have moved to nobody (or to somebody else).
//
// The id falls back to the username rather than being authoritative on its own,
// because a row is only stamped with an id once its owner has spoken since the
// column shipped — so an id miss is the expected case for a while yet, and it
// means "not stamped", not "no such user".
func resolveUser(ctx context.Context, platform, platformUserID, username string) (users.User, error) {
	if platformUserID != "" {
		u, err := findUserByID(ctx, platform, platformUserID)
		if err == nil {
			return u, nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			slog.ErrorContext(ctx, "error finding user by platform id", "err", err,
				"platform_user_id", platformUserID)
		}
	}
	if username == "" {
		return users.User{}, gorm.ErrRecordNotFound
	}
	return findUser(ctx, platform, username)
}

// gatherUserProfile reads a chatter's at-a-glance stats through the DB seams,
// scoped to this instance's platform. Operator-triggered (one click), not a
// hot path, so the extra monthly-score query is fine. No matching row — or
// neither identifier given — returns Found=false.
//
// platformUserID may be empty; see resolveUser for how the two identifiers rank.
func gatherUserProfile(ctx context.Context, platform, platformUserID, username string) userProfile {
	username = strings.ToLower(strings.TrimSpace(username))
	platformUserID = strings.TrimSpace(platformUserID)
	prof := userProfile{Username: username}
	if username == "" && platformUserID == "" {
		return prof
	}
	u, err := resolveUser(ctx, platform, platformUserID, username)
	if err != nil {
		// not-found renders as Found=false; a real DB error does too, but
		// gets logged so it's visible as a failure rather than a ghost user.
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			slog.ErrorContext(ctx, "error finding user", "err", err, "username", username)
		}
		return prof
	}
	// The stored name wins over the requested one: resolving by id after a
	// rename is exactly the case where they differ, and the events below are
	// keyed by the name they were written under.
	prof.Username = u.Username
	prof.Found = true
	prof.IsBot = u.IsBot
	prof.Miles = u.Miles
	prof.MonthlyMiles = monthlyMiles(ctx, u)
	prof.FirstSeen = bestEffortFirstSeen(u.FirstSeen, u.DateCreated, earliestEvent(ctx, u.Platform, u.Username))
	prof.LastSeen = u.LastSeen
	prof.Sessions = sessionCount(ctx, platform, u.Username)
	return prof
}

// userProfileAPIHandler serves GET /api/user/{username}: a chatter's
// at-a-glance stats as JSON, for the standalone tripbot-console to render its
// own popover (the console holds no DB access — it proxies here).
//
// An optional ?user_id= carries the platform's own user id, which resolves the
// chatter across a rename. It's a query parameter rather than a second route so
// a caller can send both and get the id's accuracy with the name's coverage;
// the response's username field says which row actually answered.
func (s *Server) userProfileAPIHandler(w http.ResponseWriter, r *http.Request) {
	prof := gatherUserProfile(r.Context(), s.cfg.Platform,
		r.URL.Query().Get("user_id"), mux.Vars(r)["username"])
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(prof); err != nil {
		slog.ErrorContext(r.Context(), "couldn't encode user profile", "err", err)
	}
}
