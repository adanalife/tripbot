package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	"gorm.io/gorm"

	"github.com/adanalife/tripbot/pkg/database"
)

// The presence report ranks accounts by connected time, for spotting the idle
// accounts that farm the miles leaderboard: miles are earned by presence
// alone, so a farm is exactly an account at the top of this list that no one
// recognizes. Unlike the other insights it keeps bots and excluded accounts in
// and reports their flags, so a reviewer can see which of the top rows are
// already handled.
const (
	presenceDefaultDays  = 7
	presenceDefaultLimit = 50
	presenceMaxLimit     = 200
	// presenceLookbackDays is how far before the window a session may have
	// begun and still count: its time inside the window is what's reported.
	presenceLookbackDays = 7
)

// presenceRow is one account in GET /api/insights/presence.
type presenceRow struct {
	Platform string `json:"platform"`
	Username string `json:"username"`
	// PresenceHours is connected time inside the window, summed over closed
	// sessions. A session that began before the window counts from its start.
	PresenceHours float64 `json:"presence_hours"`
	// Sessions counts the closed sessions ending inside the window.
	Sessions int `json:"sessions"`
	// LongestSessionHours is the longest of those, measured end to end.
	LongestSessionHours    float64 `json:"longest_session_hours"`
	IsBot                  bool    `json:"is_bot"`
	ExcludeFromLeaderboard bool    `json:"exclude_from_leaderboard"`
}

// presenceResponse is the wire shape of GET /api/insights/presence.
type presenceResponse struct {
	Days     int           `json:"days"`
	Accounts []presenceRow `json:"accounts"`
}

// presenceSQL pairs each logout in the window with the login carrying its
// session_id. The login side is windowed too (by the lookback), so both sides
// scan events_event_date rather than the whole table. The all-zero UUID is
// what a row without a session writes.
//
// ponytail: closed sessions only — a session still open at request time (or
// orphaned by a crash) has no logout to measure against. The session_over_24h
// event covers a long one still in progress.
const presenceSQL = `
SELECT o.platform,
       o.username,
       ROUND((SUM(EXTRACT(EPOCH FROM (o.date_created -
             GREATEST(l.date_created, now() - make_interval(days => @days))))) / 3600.0)::numeric, 2)::float8 AS presence_hours,
       COUNT(*) AS sessions,
       ROUND((MAX(EXTRACT(EPOCH FROM (o.date_created - l.date_created))) / 3600.0)::numeric, 2)::float8 AS longest_session_hours,
       u.is_bot,
       u.exclude_from_leaderboard
FROM events o
JOIN events l ON l.session_id = o.session_id
             AND l.event = 'login'
             AND l.date_created >= now() - make_interval(days => @since)
JOIN users u  ON u.platform = o.platform AND u.username = o.username
WHERE o.event = 'logout'
  AND o.session_id IS NOT NULL
  AND o.session_id <> '00000000-0000-0000-0000-000000000000'
  AND o.date_created >= now() - make_interval(days => @days)
  AND o.date_created >= l.date_created
GROUP BY o.platform, o.username, u.is_bot, u.exclude_from_leaderboard
ORDER BY presence_hours DESC, o.platform, o.username
LIMIT @limit`

func gatherPresence(ctx context.Context, days, limit int) (presenceResponse, error) {
	out := presenceResponse{Days: days, Accounts: []presenceRow{}}
	if err := database.GormDB().WithContext(ctx).
		Raw(presenceSQL,
			sql.Named("days", days),
			sql.Named("since", days+presenceLookbackDays),
			sql.Named("limit", limit)).
		Scan(&out.Accounts).Error; err != nil {
		return out, fmt.Errorf("presence: %w", err)
	}
	return out, nil
}

// presenceInsightsHandler serves GET /api/insights/presence: the accounts with
// the most connected time over the ?days window, top ?limit first.
func presenceInsightsHandler(w http.ResponseWriter, r *http.Request) {
	days := insightsDays(r, presenceDefaultDays)
	limit := queryInt(r, "limit", presenceDefaultLimit, 1, presenceMaxLimit)
	payload, err := gatherPresence(r.Context(), days, limit)
	if err != nil {
		slog.ErrorContext(r.Context(), "presence insights query failed", "err", err, "days", days)
		insightsError(w, "couldn't gather presence insights")
		return
	}
	writeInsights(w, r, payload)
}

// UserFlagger writes the two account flags the presence report surfaces.
// *users.Sessions implements it, which keeps the live session copy in step:
// a DB-only is_bot write would be reverted by the next miles checkpoint's
// save of a logged-in viewer.
type UserFlagger interface {
	SetBot(ctx context.Context, username string, isBot bool) error
	SetExcludeFromLeaderboard(ctx context.Context, username string, exclude bool) error
}

// SetUserFlags injects the flag writer backing POST /api/user/{username}/flags.
// Called from cmd/tripbot before Start runs the HTTP server (so there's no race
// on the field). Left nil, the endpoint answers 503.
func (s *Server) SetUserFlags(f UserFlagger) {
	s.userFlags = f
}

// userFlagsHandler serves POST /api/user/{username}/flags. The body sets
// either flag or both — {"is_bot": true}, {"exclude_from_leaderboard": false}
// — for the account on this instance's platform, so the console sends it to
// the instance matching the report row's platform. An account with no row on
// this platform is a 404. The response echoes what was written.
func (s *Server) userFlagsHandler(w http.ResponseWriter, r *http.Request) {
	if s.userFlags == nil {
		http.Error(w, "user flags unavailable", http.StatusServiceUnavailable)
		return
	}
	username := strings.ToLower(strings.TrimSpace(mux.Vars(r)["username"]))
	var body struct {
		IsBot                  *bool `json:"is_bot"`
		ExcludeFromLeaderboard *bool `json:"exclude_from_leaderboard"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || (body.IsBot == nil && body.ExcludeFromLeaderboard == nil) {
		http.Error(w, "body must set is_bot or exclude_from_leaderboard", http.StatusBadRequest)
		return
	}
	var err error
	if body.IsBot != nil {
		err = s.userFlags.SetBot(r.Context(), username, *body.IsBot)
	}
	if err == nil && body.ExcludeFromLeaderboard != nil {
		err = s.userFlags.SetExcludeFromLeaderboard(r.Context(), username, *body.ExcludeFromLeaderboard)
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		http.Error(w, "no such user on this platform", http.StatusNotFound)
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "user flag write failed", "err", err, "username", username)
		http.Error(w, "couldn't write user flags", http.StatusInternalServerError)
		return
	}
	slog.InfoContext(r.Context(), "user flags set via console", "username", username,
		"is_bot", body.IsBot, "exclude_from_leaderboard", body.ExcludeFromLeaderboard)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":                       true,
		"platform":                 s.cfg.Platform,
		"username":                 username,
		"is_bot":                   body.IsBot,
		"exclude_from_leaderboard": body.ExcludeFromLeaderboard,
	})
}
