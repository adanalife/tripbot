package server

import (
	"context"
	"strings"
	"testing"
	"time"

	mytwitch "github.com/adanalife/tripbot/pkg/twitch"
)

func TestRenderAuthCard(t *testing.T) {
	exp := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
	got := renderAuthCard([]mytwitch.AccountTokenStatus{
		{Account: "bot", LoginAs: "tripbot4000", ExpiresAt: exp},
		{Account: "broadcaster", LoginAs: "adanalife_", Reason: "missing", InitURL: "https://x/auth/init?account=broadcaster"},
	})

	// Healthy row: an .auth-expires span carrying the expiry as data-expires
	// (Unix seconds) for the JS countdown.
	if !strings.Contains(got, `class="auth-expires" data-expires="`+itoa(int(exp.Unix()))+`"`) {
		t.Errorf("healthy row missing data-expires: %q", got)
	}
	if !strings.Contains(got, ">bot<") {
		t.Errorf("missing bot label: %q", got)
	}
	// Unhealthy row: a re-auth link and the warn class instead of a countdown.
	if !strings.Contains(got, `class="auth-reauth"`) || !strings.Contains(got, "auth-warn") {
		t.Errorf("unhealthy row missing reauth link/warn: %q", got)
	}
}

func TestRenderReauthCallout(t *testing.T) {
	// No account needs re-auth → empty string, so the live #reauth-card clears.
	if got := renderReauthCallout(nil); got != "" {
		t.Errorf("healthy → empty callout, got %q", got)
	}
	got := renderReauthCallout([]mytwitch.AccountReauth{
		{Account: "bot", LoginAs: "tripbot4000", Reason: "missing", InitURL: "https://x/auth/init"},
	})
	if !strings.Contains(got, "action needed") || !strings.Contains(got, "Sign in as tripbot4000") {
		t.Errorf("callout missing expected content: %q", got)
	}
}

func TestReauthsFromStatuses(t *testing.T) {
	out := reauthsFromStatuses([]mytwitch.AccountTokenStatus{
		{Account: "bot", Reason: ""}, // healthy → excluded
		{Account: "broadcaster", LoginAs: "adanalife_", Reason: "expired", InitURL: "u"},
	})
	if len(out) != 1 || out[0].Account != "broadcaster" || out[0].Reason != "expired" {
		t.Fatalf("got %+v, want only the broadcaster (expired)", out)
	}
}

func TestHub_pollAuth_pushesAuthAndReauthOnStart(t *testing.T) {
	saved := tokenStatusesFn
	tokenStatusesFn = func() []mytwitch.AccountTokenStatus {
		return []mytwitch.AccountTokenStatus{
			{Account: "bot", LoginAs: "tripbot4000", Reason: "missing", InitURL: "u"},
		}
	}
	t.Cleanup(func() { tokenStatusesFn = saved })

	h := NewHub()
	client := h.register()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go h.pollAuth(ctx)

	// pollAuth pushes both an "auth" and a "reauth" event immediately on start.
	got := map[string]string{}
	for i := 0; i < 2; i++ {
		select {
		case ev := <-client:
			got[ev.Name] = ev.Data
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for auth events; got %v", got)
		}
	}
	if _, ok := got["auth"]; !ok {
		t.Errorf("no auth event pushed on start")
	}
	if !strings.Contains(got["reauth"], "action needed") {
		t.Errorf("reauth event missing callout for a missing token: %q", got["reauth"])
	}
}
