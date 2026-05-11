// cmd/auth-bootstrap is the one-time interactive Twitch OAuth bootstrap.
// Runs locally (on Dana's laptop) against the cluster Postgres via port-forward
// to populate the oauth_tokens row that the cluster tripbot pod consumes at
// boot.
//
// Flow:
//   1. Verify DB reachable.
//   2. Generate CSRF state, build authorize URL with mytwitch.Scopes.
//   3. Spin up a tiny localhost:8080 HTTP listener for the OAuth callback.
//   4. Open the browser to the authorize URL. Dana signs in as tripbot4000.
//   5. Twitch redirects to localhost:8080/auth/callback. The handler validates
//      state, exchanges the code via mytwitch.GenerateUserAccessToken (which
//      Upserts the row), and signals completion.
//   6. Exit cleanly.
//
// The Twitch app's registered redirect URI is http://localhost:8080/auth/callback;
// this CLI relies on that registration matching what helix.Client sends.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	_ "github.com/adanalife/tripbot/pkg/config/tripbot" // env loader via package init
	"github.com/adanalife/tripbot/pkg/database"
	"github.com/adanalife/tripbot/pkg/helpers"
	"github.com/adanalife/tripbot/pkg/server/oauthstate"
	mytwitch "github.com/adanalife/tripbot/pkg/twitch"
	"github.com/logrusorgru/aurora"
	"github.com/nicklaw5/helix"
)

const (
	listenAddr     = "localhost:8080"
	callbackPath   = "/auth/callback"
	flowTimeout    = 5 * time.Minute
	shutdownGrace  = 5 * time.Second
	successHTML    = `<!doctype html><html><head><meta charset="utf-8"><title>tripbot — bootstrap success</title><style>body{background:#0a0a0a;color:#eee;font:14px/1.5 -apple-system,monospace;display:flex;align-items:center;justify-content:center;height:100vh;margin:0}</style></head><body><div><h1>Success</h1><p>Refresh token persisted. You may close this tab.</p></div></body></html>`
)

func main() {
	// Verify the DB is reachable before any user-facing work; if the
	// port-forward isn't up this is where it surfaces.
	if database.Connection() == nil {
		log.Fatal("could not connect to database; ensure DATABASE_HOST/USER/PASS/DB are set and a port-forward to the cluster Postgres is up (task tripbot:db:up)")
	}

	client, err := mytwitch.Client()
	if err != nil {
		log.Fatalf("could not build Twitch client: %v", err)
	}

	state := oauthstate.New()
	authURL := client.GetAuthorizationURL(&helix.AuthorizationURLParams{
		Scopes:       mytwitch.Scopes,
		ResponseType: "code",
		State:        state,
	})

	done := make(chan error, 1)
	mux := http.NewServeMux()
	mux.HandleFunc(callbackPath, func(w http.ResponseWriter, r *http.Request) {
		if !oauthstate.Validate(r.URL.Query().Get("state")) {
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
		if err := mytwitch.GenerateUserAccessToken(code); err != nil {
			http.Error(w, "code exchange failed: "+err.Error(), http.StatusInternalServerError)
			done <- err
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, successHTML)
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

	log.Println(aurora.Cyan("Opening browser for Twitch sign-in..."))
	log.Println("If your browser doesn't open automatically, visit:")
	log.Println(aurora.Blue(authURL).Underline())
	helpers.OpenInBrowser(authURL)

	select {
	case err := <-done:
		if err != nil {
			log.Fatalf("bootstrap failed: %v", err)
		}
		log.Println(aurora.Green("bootstrap successful — refresh token persisted to oauth_tokens"))
	case <-time.After(flowTimeout):
		log.Fatalf("timed out after %s waiting for Twitch callback", flowTimeout)
	}
}
