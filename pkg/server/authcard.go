package server

import (
	"context"
	"html/template"
	"log/slog"
	"strings"
	"time"

	mytwitch "github.com/adanalife/tripbot/pkg/twitch"
)

// authPollInterval is how often the hub re-reads token state and pushes a fresh
// auth card + reauth callout. Token expiry moves on the order of minutes/hours,
// so 30s is plenty; the page's JS ticker counts the displayed "expires in N"
// down between polls.
const authPollInterval = 30 * time.Second

// tokenStatusesFn is the seam the hub polls for live token state. This is
// pull-based — the documented exception to the hub's otherwise NATS-only
// sourcing (see the admin-live-console ADR), mirroring the admin page's reauth
// seam. Reads in-memory token state, so it's cheap to poll. Overridable in tests.
var tokenStatusesFn = mytwitch.TokenStatuses

// pollAuth pushes the auth card + reauth callout on start and every
// authPollInterval thereafter, until ctx is done. Pull-based, so it runs even
// when NATS is unconfigured (the SSE clients exist regardless of NATS). A page
// loaded between polls renders its initial auth state server-side (see
// adminData.AuthStatuses), so missing the retroactive push is harmless.
func (h *Hub) pollAuth(ctx context.Context) {
	push := func() {
		statuses := tokenStatusesFn()
		h.broadcast(sseEvent{Name: "auth", Data: renderAuthCard(statuses)})
		h.broadcast(sseEvent{Name: "reauth", Data: renderReauthCallout(reauthsFromStatuses(statuses))})
	}
	push()
	t := time.NewTicker(authPollInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			push()
		}
	}
}

// reauthsFromStatuses narrows the full per-identity status list to just the
// accounts that need operator re-auth, in the shape the callout renders.
func reauthsFromStatuses(statuses []mytwitch.AccountTokenStatus) []mytwitch.AccountReauth {
	var out []mytwitch.AccountReauth
	for _, s := range statuses {
		if s.Reason != "" {
			out = append(out, mytwitch.AccountReauth{
				Account: s.Account, LoginAs: s.LoginAs, Reason: s.Reason, InitURL: s.InitURL,
			})
		}
	}
	return out
}

// authCardTmpl renders one inline row per identity: the account label plus
// either a re-auth link (when the token is missing/expired) or an .auth-expires
// span carrying the expiry as data-since-style data-expires (Unix seconds) that
// the page's JS counts down. Single source of truth — the initial server render
// (admin.go's authCard FuncMap) and the live hub push both go through here.
var authCardTmpl = template.Must(template.New("authcard").Parse(
	`{{range .}}<span class="auth-row{{if .Reason}} auth-warn{{end}}"><span class="auth-who">{{.Account}}</span> ` +
		`{{if .Reason}}<a class="auth-reauth" href="{{.InitURL}}">re-authenticate</a>` +
		`{{else}}<span class="auth-expires" data-expires="{{.ExpiresAt.Unix}}">…</span>{{end}}</span> {{end}}`))

func renderAuthCard(statuses []mytwitch.AccountTokenStatus) string {
	var sb strings.Builder
	if err := authCardTmpl.Execute(&sb, statuses); err != nil {
		slog.Error("live-console hub: render auth card", "err", err)
		return ""
	}
	return sb.String()
}

// reauthCalloutTmpl renders the prominent "action needed" callout, or nothing
// when no account needs re-auth (so the live #reauth-card empties out and the
// banner disappears without a reload). Markup mirrors the server-rendered
// version this replaces.
var reauthCalloutTmpl = template.Must(template.New("reauthcallout").Parse(
	`{{if .}}<div class="reauth"><h2>action needed: re-authenticate</h2>` +
		`<p>tripbot can't talk to Twitch until these accounts are re-authorized. Sign in as the named account on each — the flow re-prompts which account to use, so sign out of Twitch (or use a private window) if it grabs the wrong one.</p>` +
		`<div class="btns">{{range .}}<a class="btn" href="{{.InitURL}}">Sign in as {{.LoginAs}} <span class="why">({{.Account}} · {{.Reason}})</span></a>{{end}}</div></div>{{end}}`))

func renderReauthCallout(reauths []mytwitch.AccountReauth) string {
	var sb strings.Builder
	if err := reauthCalloutTmpl.Execute(&sb, reauths); err != nil {
		slog.Error("live-console hub: render reauth callout", "err", err)
		return ""
	}
	return sb.String()
}
