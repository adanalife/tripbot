package twitch

import (
	"testing"
	"time"

	"github.com/adanalife/tripbot/pkg/oauthtokens"
)

// clientWithTokens builds a fresh *API seeded with the given bot +
// broadcaster tokens. Each test gets its own isolated client — no shared
// global to save/restore, no mutex dance for single-goroutine setup.
func clientWithTokens(bot, bcast oauthtokens.Token) *API {
	cl := New()
	cl.currentUserToken = bot
	cl.currentBroadcasterToken = bcast
	return cl
}

func TestBroadcasterUserAccessToken_RawNoPrefix(t *testing.T) {
	cl := clientWithTokens(oauthtokens.Token{}, oauthtokens.Token{AccessToken: "abc123"})
	// pkg/eventsub passes this straight into the WS handshake, which wants the
	// bare token — an "oauth:" prefix would be rejected.
	if got := cl.BroadcasterUserAccessToken(); got != "abc123" {
		t.Errorf("BroadcasterUserAccessToken() = %q, want %q", got, "abc123")
	}
}

func TestBroadcasterUserAccessToken_EmptyWhenUnloaded(t *testing.T) {
	cl := clientWithTokens(oauthtokens.Token{}, oauthtokens.Token{})
	if got := cl.BroadcasterUserAccessToken(); got != "" {
		t.Errorf("BroadcasterUserAccessToken() with empty token = %q, want \"\"", got)
	}
}

// TestNew_IsolatedState confirms two constructed clients don't share token
// state — each *API carries its own.
func TestNew_IsolatedState(t *testing.T) {
	a := clientWithTokens(oauthtokens.Token{}, oauthtokens.Token{AccessToken: "a-tok"})
	b := New()
	if a.BroadcasterUserAccessToken() == "" {
		t.Fatal("client a should carry its seeded token")
	}
	if b.BroadcasterUserAccessToken() != "" {
		t.Errorf("client b should be empty; got %q — state leaked between instances", b.BroadcasterUserAccessToken())
	}
}

func TestErrNoToken_AliasesOAuthTokens(t *testing.T) {
	if ErrNoToken != oauthtokens.ErrNoToken {
		t.Errorf("ErrNoToken does not match oauthtokens.ErrNoToken; sentinel comparisons will fail")
	}
}

// TestTokenReads_Concurrent is a smoke check that the RWMutex doesn't deadlock
// under parallel reads.
func TestTokenReads_Concurrent(t *testing.T) {
	cl := clientWithTokens(oauthtokens.Token{}, oauthtokens.Token{AccessToken: "race-check"})

	done := make(chan struct{})
	for i := 0; i < 8; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				_ = cl.BroadcasterUserAccessToken()
			}
			done <- struct{}{}
		}()
	}
	deadline := time.After(2 * time.Second)
	for i := 0; i < 8; i++ {
		select {
		case <-done:
		case <-deadline:
			t.Fatal("concurrent token readers timed out (deadlock?)")
		}
	}
}

func TestTokenStatuses_HealthyReportsExpiryForEveryIdentity(t *testing.T) {
	botExp := time.Now().Add(3 * time.Hour)
	bcastExp := time.Now().Add(2 * time.Hour)
	cl := clientWithTokens(
		oauthtokens.Token{AccessToken: "good", ExpiresAt: botExp},
		oauthtokens.Token{AccessToken: "good", ExpiresAt: bcastExp},
	)

	got := cl.TokenStatuses("tripbot4000", "adanalife_")
	if len(got) != 2 {
		t.Fatalf("got %d statuses, want 2 (bot + broadcaster): %+v", len(got), got)
	}
	// Healthy identities are still reported (with their expiry) so the console
	// can show a countdown.
	if got[0].Account != "bot" || got[0].Reason != "" || !got[0].ExpiresAt.Equal(botExp) {
		t.Errorf("bot status = %+v, want healthy with botExp", got[0])
	}
	if got[1].Account != "broadcaster" || got[1].Reason != "" || !got[1].ExpiresAt.Equal(bcastExp) {
		t.Errorf("broadcaster status = %+v, want healthy with bcastExp", got[1])
	}
}

func TestTokenStatuses_CarriesReauthReason(t *testing.T) {
	healthy := oauthtokens.Token{AccessToken: "good", ExpiresAt: time.Now().Add(time.Hour)}
	cl := clientWithTokens(oauthtokens.Token{}, healthy) // bot blank → missing

	got := cl.TokenStatuses("tripbot4000", "adanalife_")
	if len(got) != 2 || got[0].Account != "bot" || got[0].Reason != "missing" {
		t.Fatalf("got %+v, want bot row with Reason=missing", got)
	}
}

// When the bot and broadcaster are the same account, there's no separate
// broadcaster row — a blank broadcaster slot must not produce a phantom entry.
func TestTokenStatuses_NoSeparateBroadcaster(t *testing.T) {
	healthy := oauthtokens.Token{AccessToken: "good", ExpiresAt: time.Now().Add(time.Hour)}
	cl := clientWithTokens(healthy, oauthtokens.Token{})

	got := cl.TokenStatuses("tripbot4000", "tripbot4000")
	if len(got) != 1 || got[0].Account != "bot" {
		t.Fatalf("TokenStatuses() = %+v, want only the bot row when no distinct broadcaster identity", got)
	}
}
