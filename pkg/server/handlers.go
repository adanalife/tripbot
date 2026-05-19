package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"

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
// the CSRF state, exchanges the code for an access+refresh token via helix,
// and persists the row (mytwitch.GenerateUserAccessToken handles the helix
// call + Upsert).
func authCallbackHandler(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	if !oauthstate.Validate(state) {
		http.Error(w, "invalid or expired state", http.StatusBadRequest)
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		slog.ErrorContext(r.Context(), "no code in response from twitch", "err", errors.New("code missing"))
		http.Error(w, "no code in response from twitch", http.StatusBadRequest)
		return
	}

	if err := generateUserAccessToken(code); err != nil {
		slog.ErrorContext(r.Context(), "GenerateUserAccessToken failed", "err", err)
		http.Error(w, "failed to exchange code: "+err.Error(), http.StatusInternalServerError)
		return
	}

	slog.InfoContext(r.Context(), "received token from twitch via auth callback")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, authSuccessHTML)
}

// authInitHandler kicks off an OAuth Authorization Code flow from a browser.
// Generates a state, redirects (302) to Twitch's authorize URL with the
// configured Scopes. The cluster pod serves this for emergency re-bootstrap
// when Dana isn't near a laptop; locally cmd/auth-bootstrap does its own
// equivalent without going through the public Ingress.
func authInitHandler(w http.ResponseWriter, r *http.Request) {
	client, err := helixClient()
	if err != nil {
		slog.ErrorContext(r.Context(), "helix client unavailable for /auth/init", "err", err)
		http.Error(w, "auth unavailable", http.StatusInternalServerError)
		return
	}
	state := oauthstate.New()
	authURL := client.GetAuthorizationURL(&helix.AuthorizationURLParams{
		Scopes:       mytwitch.Scopes,
		ResponseType: "code",
		State:        state,
	})
	http.Redirect(w, r, authURL, http.StatusFound)
}

// authSuccessHTML is the body returned after a successful code exchange.
// Inline so the handler doesn't depend on a template file; if this needs
// styling beyond a few lines, move to an embed.FS template.
const authSuccessHTML = `<!doctype html>
<html>
<head><meta charset="utf-8"><title>tripbot — auth success</title>
<style>body{background:#0a0a0a;color:#eee;font:14px/1.5 -apple-system,monospace;display:flex;align-items:center;justify-content:center;height:100vh;margin:0}</style>
</head>
<body><div><h1>Success</h1><p>You may close this tab.</p></div></body>
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
