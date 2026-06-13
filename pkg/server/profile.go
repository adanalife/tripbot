package server

import (
	"context"
	"html/template"
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
// a chatter, events-derived per the tripbot-events-table-design ADR.
type userProfile struct {
	Username     string
	Found        bool
	IsBot        bool
	Miles        float32 // lifetime
	MonthlyMiles float32 // current month
	Sessions     int64
	FirstSeen    time.Time
	LastSeen     time.Time
}

// userProfileHandler serves GET /admin/user/{username}: the HTML fragment the
// live console pops over when an operator clicks a username. Timeout/ban
// actions are planned here (they need the broadcaster token's
// moderator:manage:banned_users scope).
func userProfileHandler(w http.ResponseWriter, r *http.Request) {
	username := strings.ToLower(strings.TrimSpace(mux.Vars(r)["username"]))
	prof := userProfile{Username: username}
	if username != "" {
		if u := findUser(r.Context(), username); u.ID != 0 {
			prof.Found = true
			prof.IsBot = u.IsBot
			prof.Miles = u.Miles
			prof.MonthlyMiles = monthlyMiles(r.Context(), u)
			prof.FirstSeen = u.DateCreated
			prof.LastSeen = u.LastSeen
			prof.Sessions = sessionCount(r.Context(), username)
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := userProfileTmpl.Execute(w, prof); err != nil {
		slog.ErrorContext(r.Context(), "couldn't render user profile", "err", err)
	}
}

// userProfileTmpl renders the popover fragment. html/template escapes the
// username everywhere it appears (incl. the Twitch URL).
var userProfileTmpl = template.Must(template.New("profile").Parse(`<div class="profile-card">
  <div class="profile-name">{{.Username}}{{if .IsBot}} <span class="profile-bot">bot</span>{{end}}</div>
  {{- if .Found}}
  <dl class="profile-stats">
    <dt>miles</dt><dd>{{printf "%.1f" .Miles}}</dd>
    <dt>this month</dt><dd>{{printf "%.1f" .MonthlyMiles}}</dd>
    <dt>sessions</dt><dd>{{.Sessions}}</dd>
    <dt>first seen</dt><dd>{{if .FirstSeen.IsZero}}unknown{{else}}{{.FirstSeen.Format "2006-01-02"}}{{end}}</dd>
    <dt>last seen</dt><dd>{{if .LastSeen.IsZero}}unknown{{else}}{{.LastSeen.Format "2006-01-02 15:04"}}{{end}}</dd>
  </dl>
  {{- else}}
  <p class="profile-empty">no record for this user yet</p>
  {{- end}}
  <a class="profile-link" href="https://twitch.tv/{{.Username}}" target="_blank" rel="noopener">view on Twitch ↗</a>
</div>`))
