package twitch

import (
	"testing"
	"time"

	"github.com/adanalife/tripbot/pkg/oauthtokens"
)

// resetToken restores currentUserToken after a test mutation.
func resetToken(t *testing.T) {
	t.Helper()
	saved := currentUserToken
	t.Cleanup(func() {
		tokenMu.Lock()
		currentUserToken = saved
		tokenMu.Unlock()
	})
}

func TestIRCAuthToken_PrefixesOauth(t *testing.T) {
	resetToken(t)
	tokenMu.Lock()
	currentUserToken = oauthtokens.Token{AccessToken: "abc123"}
	tokenMu.Unlock()

	got := IRCAuthToken()
	if got != "oauth:abc123" {
		t.Errorf("IRCAuthToken() = %q, want %q", got, "oauth:abc123")
	}
}

func TestIRCAuthToken_EmptyWhenUnloaded(t *testing.T) {
	resetToken(t)
	tokenMu.Lock()
	currentUserToken = oauthtokens.Token{}
	tokenMu.Unlock()

	if got := IRCAuthToken(); got != "" {
		t.Errorf("IRCAuthToken() with empty token = %q, want \"\"", got)
	}
}

func TestCurrentUserAccessToken_ReturnsRaw(t *testing.T) {
	resetToken(t)
	tokenMu.Lock()
	currentUserToken = oauthtokens.Token{AccessToken: "raw-no-prefix"}
	tokenMu.Unlock()

	if got := CurrentUserAccessToken(); got != "raw-no-prefix" {
		t.Errorf("CurrentUserAccessToken() = %q, want %q", got, "raw-no-prefix")
	}
}

func TestScopes_IncludesIRCScopes(t *testing.T) {
	required := []string{"chat:read", "chat:edit"}
	have := map[string]bool{}
	for _, s := range Scopes {
		have[s] = true
	}
	for _, r := range required {
		if !have[r] {
			t.Errorf("Scopes missing required IRC scope %q (have %v)", r, Scopes)
		}
	}
}

func TestScopes_NoDuplicates(t *testing.T) {
	seen := map[string]bool{}
	for _, s := range Scopes {
		if seen[s] {
			t.Errorf("duplicate scope %q in Scopes", s)
		}
		seen[s] = true
	}
}

func TestScopes_DropsOpenID(t *testing.T) {
	// openid was in the previous scope set but the bot doesn't read ID
	// claims; dropping it shrinks the consent screen and reduces surface.
	for _, s := range Scopes {
		if s == "openid" {
			t.Errorf("Scopes still includes openid; expected drop")
		}
	}
}

func TestErrNoToken_AliasesOAuthTokens(t *testing.T) {
	if ErrNoToken != oauthtokens.ErrNoToken {
		t.Errorf("ErrNoToken does not match oauthtokens.ErrNoToken; sentinel comparisons will fail")
	}
}

// TestIRCAuthToken_ConcurrentReads is a smoke check that the RWMutex doesn't
// deadlock under parallel reads while a writer takes the lock.
func TestIRCAuthToken_ConcurrentReads(t *testing.T) {
	resetToken(t)
	tokenMu.Lock()
	currentUserToken = oauthtokens.Token{AccessToken: "race-check"}
	tokenMu.Unlock()

	done := make(chan struct{})
	for i := 0; i < 8; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				_ = IRCAuthToken()
			}
			done <- struct{}{}
		}()
	}
	deadline := time.After(2 * time.Second)
	for i := 0; i < 8; i++ {
		select {
		case <-done:
		case <-deadline:
			t.Fatal("concurrent IRCAuthToken readers timed out (deadlock?)")
		}
	}
}
