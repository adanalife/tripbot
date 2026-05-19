// cmd/auth-bootstrap is the interactive Twitch OAuth bootstrap.
// Runs locally (on Dana's laptop) against the cluster Postgres via port-forward
// to populate oauth_tokens rows that the cluster tripbot pod consumes at boot.
//
// Two identities need separate consent — the bot (chat:read/edit IRC) and
// the broadcaster (channel:read:subscriptions etc.). Pick one per run via
// --account=bot|broadcaster.
//
// Flow:
//   1. Verify DB reachable.
//   2. Generate CSRF state, build authorize URL with BotScopes or
//      BroadcasterScopes per --account. ForceVerify=true so Twitch re-prompts
//      which account to sign in as (instead of silently reusing the session
//      cookie from the previous leg).
//   3. Spin up a tiny localhost:8080 HTTP listener for the OAuth callback.
//   4. Open the browser to the authorize URL. Sign in as the matching account.
//   5. Twitch redirects to localhost:8080/auth/callback. The handler validates
//      state, exchanges the code via mytwitch.GenerateUserAccessToken (which
//      Upserts the row keyed by whichever account signed in), and signals
//      completion.
//   6. Exit cleanly.
//
// The Twitch app's registered redirect URI is http://localhost:8080/auth/callback;
// this CLI relies on that registration matching what helix.Client sends.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"runtime"
	"time"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/database"
	"github.com/adanalife/tripbot/pkg/helpers"
	"github.com/adanalife/tripbot/pkg/server/oauthstate"
	mytwitch "github.com/adanalife/tripbot/pkg/twitch"
	"github.com/nicklaw5/helix/v2"
)

const (
	listenAddr    = "localhost:8080"
	callbackPath  = "/auth/callback"
	flowTimeout   = 5 * time.Minute
	shutdownGrace = 5 * time.Second
	// %s = expectedLogin, %s = --account flag value
	successHTML = `<!doctype html><html><head><meta charset="utf-8"><title>tripbot — bootstrap success</title><style>body{background:#0a0a0a;color:#eee;font:14px/1.5 -apple-system,monospace;display:flex;align-items:center;justify-content:center;height:100vh;margin:0}</style></head><body><div><h1>Success</h1><p>Refresh token persisted for <strong>%s</strong> (%s account). You may close this tab.</p></div></body></html>`
	// %s = got login, %s = expected login, %s = expected account ("bot"/"broadcaster")
	mismatchHTML = `<!doctype html><html><head><meta charset="utf-8"><title>tripbot — wrong account</title><style>body{background:#3a0000;color:#fff;font:14px/1.5 -apple-system,monospace;display:flex;align-items:center;justify-content:center;height:100vh;margin:0}div{max-width:520px}code{background:#000;padding:2px 5px;border-radius:3px}</style></head><body><div><h1>Wrong account</h1><p>You signed in as <code>%s</code>, but this leg expected <code>%s</code> (the <strong>%s</strong> account).</p><p>No token was written. Sign out of Twitch in this browser (or open an incognito window), then re-run the bootstrap task.</p></div></body></html>`
)

func main() {
	account := flag.String("account", "bot", "which account to bootstrap: bot (chat:read/edit, etc.) or broadcaster (channel:read:subscriptions, moderator:read:followers, etc.)")
	flag.Parse()

	var (
		scopes        []string
		expectedLogin string
		stateAccount  oauthstate.Account
	)
	switch *account {
	case "bot":
		scopes = mytwitch.BotScopes
		expectedLogin = c.Conf.BotUsername
		stateAccount = oauthstate.AccountBot
	case "broadcaster":
		scopes = mytwitch.BroadcasterScopes
		expectedLogin = c.Conf.ChannelName
		stateAccount = oauthstate.AccountBroadcaster
	default:
		log.Fatalf("--account must be 'bot' or 'broadcaster', got %q", *account)
	}

	// Verify the DB is reachable before any user-facing work; if the
	// port-forward isn't up this is where it surfaces.
	if database.Connection() == nil {
		log.Fatal("could not connect to database; ensure DATABASE_HOST/USER/PASS/DB are set and a port-forward to the cluster Postgres is up (task tripbot:db:up)")
	}

	client, err := mytwitch.Client()
	if err != nil {
		log.Fatalf("could not build Twitch client: %v", err)
	}

	state := oauthstate.New(stateAccount)
	authURL := client.GetAuthorizationURL(&helix.AuthorizationURLParams{
		Scopes:       scopes,
		ResponseType: "code",
		// Force-verify makes Twitch re-prompt which account to sign in as,
		// rather than silently reusing the session cookie from a prior leg.
		// Without this, running bot then broadcaster back-to-back would just
		// re-bootstrap whichever account Twitch's session is on.
		ForceVerify: true,
		State:       state,
	})

	done := make(chan error, 1)
	mux := http.NewServeMux()
	mux.HandleFunc(callbackPath, func(w http.ResponseWriter, r *http.Request) {
		if _, ok := oauthstate.Validate(r.URL.Query().Get("state")); !ok {
			http.Error(w, "invalid or expired state", http.StatusBadRequest)
			done <- errors.New("state validation failed")
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			http.Error(w, "no code in response from twitch", http.StatusBadRequest)
			done <- errors.New("no code")
			return
		}
		if err := mytwitch.GenerateUserAccessToken(code, expectedLogin); err != nil {
			var mismatch *mytwitch.ErrIdentityMismatch
			if errors.As(err, &mismatch) {
				// Friendly browser page; no row was written.
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.WriteHeader(http.StatusBadRequest)
				fmt.Fprintf(w, mismatchHTML, mismatch.Got, mismatch.Expected, mismatch.AccountID)
			} else {
				http.Error(w, "code exchange failed: "+err.Error(), http.StatusInternalServerError)
			}
			done <- err
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, successHTML, expectedLogin, *account)
		done <- nil
	})

	srv := &http.Server{Addr: listenAddr, Handler: mux}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			done <- fmt.Errorf("listener: %w", err)
		}
	}()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	slog.Info("opening browser for Twitch sign-in", "account", *account)
	slog.Info("visit URL", "url", authURL)
	// Skip browser open in headless environments (e.g. the k8s Job).
	if os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != "" || runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
		helpers.OpenInBrowser(authURL)
	}

	select {
	case err := <-done:
		if err != nil {
			var mismatch *mytwitch.ErrIdentityMismatch
			if errors.As(err, &mismatch) {
				log.Fatalf("bootstrap failed: %s\nNo token was written. Sign out of Twitch (or use a fresh incognito browser) and re-run `task auth:bootstrap:%s`.", mismatch.Error(), *account)
			}
			log.Fatalf("bootstrap failed: %v", err)
		}
		slog.Info("bootstrap successful, refresh token persisted to oauth_tokens", "login", expectedLogin, "account", *account)
	case <-time.After(flowTimeout):
		log.Fatalf("timed out after %s waiting for Twitch callback", flowTimeout)
	}
}
