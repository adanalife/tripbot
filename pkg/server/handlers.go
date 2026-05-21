package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"sync/atomic"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
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

// versionTag is set by main via SetVersion; overridden at build time
// through `-ldflags "-X main.version=..."`.
var versionTag = "dev"

// SetVersion lets cmd/tripbot inject its build-time version string
// before the HTTP server starts.
func SetVersion(v string) {
	if v != "" {
		versionTag = v
	}
}

// ready reports whether the bot has established its Twitch IRC connection.
// /health/ready returns 503 until it flips true, so the orchestrator keeps
// the pod running (no crashloop) but marks it not-live until Twitch is
// reachable. cmd/tripbot flips it via SetReady on IRC connect / disconnect.
var ready atomic.Bool

// SetReady updates the readiness state reported by /health/ready.
func SetReady(r bool) {
	ready.Store(r)
}

// liveHandler is the liveness probe: the process is up and serving HTTP.
// Always 200 — a failing liveness probe restarts the pod, which is exactly
// what we want to avoid while waiting on Twitch.
func liveHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "OK")
}

// readyHandler is the readiness probe: 200 once the Twitch IRC connection is
// established, 503 otherwise. Keeps the pod up but not-live until Twitch is
// reachable, and recovers on its own once the connection lands.
func readyHandler(w http.ResponseWriter, r *http.Request) {
	if !ready.Load() {
		http.Error(w, "not ready: awaiting Twitch connection", http.StatusServiceUnavailable)
		return
	}
	fmt.Fprintf(w, "OK")
}

// versionHandler returns build metadata as JSON. The tag comes from the
// build-time ldflag; sha + built_at are read from the binary's embedded
// VCS info (Go's automatic -buildvcs).
func versionHandler(w http.ResponseWriter, r *http.Request) {
	resp := struct {
		Tag     string `json:"tag"`
		Sha     string `json:"sha"`
		BuiltAt string `json:"built_at"`
	}{Tag: versionTag}

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
		slog.ErrorContext(r.Context(), "no code in response from twitch", "err", errors.New("code missing"))
		http.Error(w, "no code in response from twitch", http.StatusBadRequest)
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
