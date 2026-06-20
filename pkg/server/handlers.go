package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/feature"
	"github.com/adanalife/tripbot/pkg/server/oauthstate"
	mytwitch "github.com/adanalife/tripbot/pkg/twitch"
	"github.com/nicklaw5/helix/v2"
)

// generateUserAccessToken is overridable in tests so the /auth/callback
// happy path can be exercised without round-tripping to Twitch.
var generateUserAccessToken = mytwitch.GenerateUserAccessToken

// helixClient is overridable in tests so /auth/init's URL-construction can be
// exercised without triggering mytwitch.Client()'s lazy network init (which
// would request a real App Access Token from Twitch).
var helixClient = mytwitch.Client

// versionTag (Server.versionTag) is set by main via SetVersion; overridden at
// build time through `-ldflags "-X main.version=..."`.

// SetVersion lets cmd/tripbot inject its build-time version string
// before the HTTP server starts.
func (s *Server) SetVersion(v string) {
	if v != "" {
		s.versionTag = v
	}
}

// SetFlags injects the feature-flag client backing the console's /api/flags
// endpoints. Called from cmd/tripbot's startFeatureFlags, before Start runs
// the HTTP server (so there's no race on the field). Left nil when the
// Postgres client fails to load — /api/flags then reports ok=false.
func (s *Server) SetFlags(fc feature.FlagClient) {
	s.flags = fc
}

// startedAt is when the process began; /version reports it so callers can derive
// uptime. (The bot's chat-connection state is surfaced separately via the
// tripbot_twitch_connected gauge, set directly by cmd/tripbot.)
var startedAt = time.Now()

// versionHandler returns build metadata as JSON. The tag comes from the
// build-time ldflag; sha + built_at are read from the binary's embedded
// VCS info (Go's automatic -buildvcs). started_at is the process start time
// (startedAt) so callers can derive uptime themselves.
func (s *Server) versionHandler(w http.ResponseWriter, r *http.Request) {
	resp := struct {
		Tag       string `json:"tag"`
		Sha       string `json:"sha"`
		BuiltAt   string `json:"built_at"`
		StartedAt string `json:"started_at"`
	}{Tag: s.versionTag, StartedAt: startedAt.UTC().Format(time.RFC3339)}

	if info, ok := debug.ReadBuildInfo(); ok {
		for _, s := range info.Settings {
			switch s.Key {
			case "vcs.revision":
				resp.Sha = s.Value
			case "vcs.time":
				resp.BuiltAt = s.Value
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		slog.ErrorContext(r.Context(), "couldn't encode version response", "err", err)
	}
}

// authCallbackHandler completes the OAuth Authorization Code flow. Validates
// the CSRF state (which round-trips the account selector from /auth/init),
// derives the expected Twitch login from the account, exchanges the code via
// helix, and persists the row (mytwitch.GenerateUserAccessToken handles the
// helix call + identity-sanity-check + Upsert).
func authCallbackHandler(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	account, ok := oauthstate.Validate(state)
	if !ok {
		http.Error(w, "invalid or expired state", http.StatusBadRequest)
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		slog.ErrorContext(r.Context(), "no code in auth callback", "err", errors.New("code missing"))
		http.Error(w, "no code in auth callback", http.StatusBadRequest)
		return
	}

	expectedLogin := ""
	switch account {
	case oauthstate.AccountBot:
		expectedLogin = c.Conf.BotUsername
	case oauthstate.AccountBroadcaster:
		expectedLogin = c.Conf.ChannelName
	}

	if err := generateUserAccessToken(code, expectedLogin); err != nil {
		var mismatch *mytwitch.ErrIdentityMismatch
		if errors.As(err, &mismatch) {
			slog.WarnContext(r.Context(), "OAuth bootstrap identity mismatch; no row written",
				"expected", mismatch.Expected, "got", mismatch.Got, "account", mismatch.AccountID)
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, authMismatchHTML, mismatch.Got, mismatch.Expected, mismatch.AccountID, mismatch.AccountID)
			return
		}
		slog.ErrorContext(r.Context(), "GenerateUserAccessToken failed", "err", err)
		http.Error(w, "failed to exchange code: "+err.Error(), http.StatusInternalServerError)
		return
	}

	slog.InfoContext(r.Context(), "received token from twitch via auth callback", "account", account, "login", expectedLogin)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, authSuccessHTML, expectedLogin, string(account))
}

// authInitHandler kicks off an OAuth Authorization Code flow from a browser.
// Generates a state, redirects (302) to Twitch's authorize URL with the
// scope set for the requested account (?account=bot|broadcaster, default bot).
// ForceVerify=true so Twitch re-prompts which account to sign in as instead
// of silently reusing the session cookie. The cluster pod serves this for
// emergency re-bootstrap when Dana isn't near a laptop; locally
// cmd/auth-bootstrap does its own equivalent without going through Ingress.
func authInitHandler(w http.ResponseWriter, r *http.Request) {
	scopes := mytwitch.BotScopes
	account := oauthstate.AccountBot
	switch r.URL.Query().Get("account") {
	case "", "bot":
		// default
	case "broadcaster":
		scopes = mytwitch.BroadcasterScopes
		account = oauthstate.AccountBroadcaster
	default:
		http.Error(w, "account must be 'bot' or 'broadcaster'", http.StatusBadRequest)
		return
	}
	client, err := helixClient()
	if err != nil {
		slog.ErrorContext(r.Context(), "helix client unavailable for /auth/init", "err", err)
		http.Error(w, "auth unavailable", http.StatusInternalServerError)
		return
	}
	state := oauthstate.New(account)
	authURL := client.GetAuthorizationURL(&helix.AuthorizationURLParams{
		Scopes:       scopes,
		ResponseType: "code",
		ForceVerify:  true,
		State:        state,
	})
	http.Redirect(w, r, authURL, http.StatusFound)
}

// authSuccessHTML is the body returned after a successful code exchange.
// Inline so the handler doesn't depend on a template file; if this needs
// styling beyond a few lines, move to an embed.FS template.
// %s = the discovered login, %s = the account ("bot"/"broadcaster").
const authSuccessHTML = `<!doctype html>
<html>
<head><meta charset="utf-8"><title>tripbot — auth success</title>
<style>body{background:#0a0a0a;color:#eee;font:14px/1.5 -apple-system,monospace;display:flex;align-items:center;justify-content:center;height:100vh;margin:0}</style>
</head>
<body><div><h1>Success</h1><p>Refresh token persisted for <strong>%s</strong> (%s account). You may close this tab.</p></div></body>
</html>`

// authMismatchHTML is returned when the discovered Twitch login doesn't
// match the expected identity for the flow. No row is written.
// %s = got login, %s = expected login, %s = account ("bot"/"broadcaster"),
// %s = account again (for the retry-link query param).
const authMismatchHTML = `<!doctype html>
<html>
<head><meta charset="utf-8"><title>tripbot — wrong account</title>
<style>body{background:#3a0000;color:#fff;font:14px/1.5 -apple-system,monospace;display:flex;align-items:center;justify-content:center;height:100vh;margin:0}div{max-width:540px}code{background:#000;padding:2px 5px;border-radius:3px}a{color:#9cf}</style>
</head>
<body><div><h1>Wrong account</h1><p>You signed in as <code>%s</code>, but this leg expected <code>%s</code> (the <strong>%s</strong> account).</p><p>No token was written. Sign out of Twitch in this browser (or open an incognito window), then <a href="/auth/init?account=%s">click here to retry</a>.</p></div></body>
</html>`

// rootHandler serves GET /: a minimal landing page linking to the bot and
// broadcaster OAuth bootstrap flows. The rich admin panel moved to the
// standalone tripbot-console; this keeps the login links reachable on the
// pod itself for emergency re-auth when Dana isn't near the console.
func rootHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := w.Write([]byte(rootHTML)); err != nil {
		slog.ErrorContext(r.Context(), "couldn't write root page", "err", err)
	}
}

// rootHTML is the body of the landing page. Inline (no template file) so the
// handler has no filesystem dependency; the two links drive /auth/init.
const rootHTML = `<!doctype html>
<html>
<head><meta charset="utf-8"><title>tripbot</title>
<style>body{background:#0a0a0a;color:#eee;font:14px/1.6 -apple-system,system-ui,sans-serif;display:flex;align-items:center;justify-content:center;min-height:100vh;margin:0}main{max-width:420px;padding:1rem}h1{font-size:1.4rem;margin:0 0 .25rem}p{color:#999;margin:0 0 1.25rem}a.login{display:block;padding:.7rem 1rem;margin:.5rem 0;background:#161616;border:1px solid #2a2a2a;border-radius:8px;color:#9cf;text-decoration:none}a.login:hover{background:#1e1e1e;border-color:#3a3a3a}.note{color:#666;font-size:12px;margin-top:1.5rem}</style>
</head>
<body><main>
<h1>tripbot</h1>
<p>OAuth bootstrap — sign in to refresh a bot token.</p>
<a class="login" href="/auth/init?account=bot">Log in the bot account →</a>
<a class="login" href="/auth/init?account=broadcaster">Log in the broadcaster account →</a>
<p class="note">The admin dashboard lives in tripbot-console.</p>
</main></body>
</html>`

// return a favicon if anyone asks for one
func faviconHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "assets/favicon.ico")
}

func catchAllHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		http.Error(w, "404 not found", http.StatusNotFound)
		slog.InfoContext(r.Context(), "404 GET", "path", r.URL.Path)
		return

	case "POST":
		// someone tried to make a post and we dont know what to do with it
		http.Error(w, "404 not found", http.StatusNotFound)
		slog.InfoContext(r.Context(), "404 POST", "path", r.URL.Path)
		return
	// someone tried a PUT or a DELETE or something
	default:
		fmt.Fprintf(w, "Only GET/POST methods are supported.\n")
	}
}
