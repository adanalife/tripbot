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

// resetBroadcasterToken restores currentBroadcasterToken after a mutation.
func resetBroadcasterToken(t *testing.T) {
	t.Helper()
	saved := currentBroadcasterToken
	t.Cleanup(func() {
		tokenMu.Lock()
		currentBroadcasterToken = saved
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

func TestBotScopes_IncludesIRCBotScopes(t *testing.T) {
	required := []string{"chat:read", "chat:edit"}
	have := map[string]bool{}
	for _, s := range BotScopes {
		have[s] = true
	}
	for _, r := range required {
		if !have[r] {
			t.Errorf("BotScopes missing required IRC scope %q (have %v)", r, BotScopes)
		}
	}
}

func TestBotScopes_NoDuplicates(t *testing.T) {
	seen := map[string]bool{}
	for _, s := range BotScopes {
		if seen[s] {
			t.Errorf("duplicate scope %q in BotScopes", s)
		}
		seen[s] = true
	}
}

func TestBotScopes_DropsOpenID(t *testing.T) {
	// openid was in the previous scope set but the bot doesn't read ID
	// claims; dropping it shrinks the consent screen and reduces surface.
	for _, s := range BotScopes {
		if s == "openid" {
			t.Errorf("BotScopes still includes openid; expected drop")
		}
	}
}

func TestBroadcasterScopes_IncludesSubscriptionsAndFollowers(t *testing.T) {
	required := []string{"channel:read:subscriptions", "moderator:read:followers"}
	have := map[string]bool{}
	for _, s := range BroadcasterScopes {
		have[s] = true
	}
	for _, r := range required {
		if !have[r] {
			t.Errorf("BroadcasterScopes missing required scope %q (have %v)", r, BroadcasterScopes)
		}
	}
}

func TestBroadcasterScopes_DisjointFromBotScopes(t *testing.T) {
	// The two scope sets serve different identities; if a scope appears in
	// both it suggests confusion about which token authorizes which call.
	bot := map[string]bool{}
	for _, s := range BotScopes {
		bot[s] = true
	}
	for _, s := range BroadcasterScopes {
		if bot[s] {
			t.Errorf("scope %q appears in both BotScopes and BroadcasterScopes", s)
		}
	}
}

func TestBroadcasterTokenLoaded_FalseWhenEmpty(t *testing.T) {
	resetBroadcasterToken(t)
	tokenMu.Lock()
	currentBroadcasterToken = oauthtokens.Token{}
	tokenMu.Unlock()

	if broadcasterTokenLoaded() {
		t.Error("broadcasterTokenLoaded() = true with empty token; want false")
	}
}

func TestBroadcasterTokenLoaded_TrueWhenSet(t *testing.T) {
	resetBroadcasterToken(t)
	tokenMu.Lock()
	currentBroadcasterToken = oauthtokens.Token{AccessToken: "broadcaster-tok"}
	tokenMu.Unlock()

	if !broadcasterTokenLoaded() {
		t.Error("broadcasterTokenLoaded() = false with token set; want true")
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
