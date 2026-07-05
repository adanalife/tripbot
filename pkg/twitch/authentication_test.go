package twitch

import (
	"errors"
	"strings"
	"testing"
	"time"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
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

func TestIRCAuthToken_PrefixesOauth(t *testing.T) {
	cl := clientWithTokens(oauthtokens.Token{AccessToken: "abc123"}, oauthtokens.Token{})
	if got := cl.IRCAuthToken(); got != "oauth:abc123" {
		t.Errorf("IRCAuthToken() = %q, want %q", got, "oauth:abc123")
	}
}

func TestIRCAuthToken_EmptyWhenUnloaded(t *testing.T) {
	cl := clientWithTokens(oauthtokens.Token{}, oauthtokens.Token{})
	if got := cl.IRCAuthToken(); got != "" {
		t.Errorf("IRCAuthToken() with empty token = %q, want \"\"", got)
	}
}

func TestCurrentUserAccessToken_ReturnsRaw(t *testing.T) {
	cl := clientWithTokens(oauthtokens.Token{AccessToken: "raw-no-prefix"}, oauthtokens.Token{})
	if got := cl.CurrentUserAccessToken(); got != "raw-no-prefix" {
		t.Errorf("CurrentUserAccessToken() = %q, want %q", got, "raw-no-prefix")
	}
}

// TestNew_IsolatedState confirms the new-shape payoff: two constructed clients
// don't share token state the way the old package globals did.
func TestNew_IsolatedState(t *testing.T) {
	a := clientWithTokens(oauthtokens.Token{AccessToken: "a-tok"}, oauthtokens.Token{})
	b := New()
	if a.IRCAuthToken() == "" {
		t.Fatal("client a should carry its seeded token")
	}
	if b.IRCAuthToken() != "" {
		t.Errorf("client b should be empty; got %q — state leaked between instances", b.IRCAuthToken())
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

func TestErrIdentityMismatch_MessageNamesBoth(t *testing.T) {
	err := &ErrIdentityMismatch{Expected: "tripbot4000", Got: "adanalife_", AccountID: "bot"}
	msg := err.Error()
	for _, want := range []string{"tripbot4000", "adanalife_", "bot"} {
		if !strings.Contains(msg, want) {
			t.Errorf("ErrIdentityMismatch.Error() = %q; missing %q", msg, want)
		}
	}
}

func TestErrIdentityMismatch_ErrorsAs(t *testing.T) {
	var orig error = &ErrIdentityMismatch{Expected: "x", Got: "y", AccountID: "broadcaster"}
	var target *ErrIdentityMismatch
	if !errors.As(orig, &target) {
		t.Fatal("errors.As did not extract *ErrIdentityMismatch")
	}
	if target.Expected != "x" || target.Got != "y" || target.AccountID != "broadcaster" {
		t.Errorf("extracted: %+v", target)
	}
}

func TestAuthInitURL_BuildsInitPath(t *testing.T) {
	got := AuthInitURL("bot")
	want := c.Conf.ExternalURL + "/auth/init?account=bot&login_as=" + c.Conf.BotUsername
	if got != want {
		t.Errorf("AuthInitURL(\"bot\") = %q, want %q", got, want)
	}
	// No CSRF state / secret should ever leak into the logged URL — it's the
	// indirection path, not the fully-formed Twitch authorize URL.
	if strings.Contains(got, "state=") || strings.Contains(got, "client_id=") {
		t.Errorf("AuthInitURL leaked a sensitive query param: %q", got)
	}
}

func TestAuthInitURL_BroadcasterAccount(t *testing.T) {
	got := AuthInitURL("broadcaster")
	want := c.Conf.ExternalURL + "/auth/init?account=broadcaster&login_as=" + c.Conf.ChannelName
	if got != want {
		t.Errorf("AuthInitURL(\"broadcaster\") = %q, want %q", got, want)
	}
}

func TestAccountLabel_BotUsernameIsBot(t *testing.T) {
	if got := accountLabel(c.Conf.BotUsername); got != "bot" {
		t.Errorf("accountLabel(bot username) = %q, want \"bot\"", got)
	}
}

func TestAccountLabel_UnknownDefaultsToBot(t *testing.T) {
	if got := accountLabel("some-unrelated-login"); got != "bot" {
		t.Errorf("accountLabel(unknown) = %q, want \"bot\"", got)
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
	cl := clientWithTokens(oauthtokens.Token{AccessToken: "race-check"}, oauthtokens.Token{})

	done := make(chan struct{})
	for i := 0; i < 8; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				_ = cl.IRCAuthToken()
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

// withReauthConf sets the bot/broadcaster identities AccountsNeedingReauth
// reads and restores them afterward.
func withReauthConf(t *testing.T, bot, channel string) {
	t.Helper()
	savedBot, savedChan := c.Conf.BotUsername, c.Conf.ChannelName
	c.Conf.BotUsername, c.Conf.ChannelName = bot, channel
	t.Cleanup(func() { c.Conf.BotUsername, c.Conf.ChannelName = savedBot, savedChan })
}

func TestAccountsNeedingReauth_AllHealthy(t *testing.T) {
	withReauthConf(t, "tripbot4000", "adanalife_")
	future := oauthtokens.Token{AccessToken: "good", ExpiresAt: time.Now().Add(time.Hour)}
	cl := clientWithTokens(future, future)

	if got := cl.AccountsNeedingReauth(); got != nil {
		t.Fatalf("AccountsNeedingReauth() = %+v, want nil when both tokens are healthy", got)
	}
}

func TestAccountsNeedingReauth_BotMissing(t *testing.T) {
	withReauthConf(t, "tripbot4000", "adanalife_")
	healthy := oauthtokens.Token{AccessToken: "good", ExpiresAt: time.Now().Add(time.Hour)}
	cl := clientWithTokens(oauthtokens.Token{}, healthy) // bot blank → missing

	got := cl.AccountsNeedingReauth()
	if len(got) != 1 {
		t.Fatalf("got %d accounts, want 1: %+v", len(got), got)
	}
	if got[0].Account != "bot" || got[0].Reason != "missing" || got[0].LoginAs != "tripbot4000" {
		t.Errorf("entry = %+v, want bot/missing/tripbot4000", got[0])
	}
	if !strings.Contains(got[0].InitURL, "account=bot") {
		t.Errorf("InitURL %q should target the bot account", got[0].InitURL)
	}
}

func TestAccountsNeedingReauth_BroadcasterExpired(t *testing.T) {
	withReauthConf(t, "tripbot4000", "adanalife_")
	healthy := oauthtokens.Token{AccessToken: "good", ExpiresAt: time.Now().Add(time.Hour)}
	expired := oauthtokens.Token{AccessToken: "stale", ExpiresAt: time.Now().Add(-time.Hour)}
	cl := clientWithTokens(healthy, expired)

	got := cl.AccountsNeedingReauth()
	if len(got) != 1 {
		t.Fatalf("got %d accounts, want 1: %+v", len(got), got)
	}
	if got[0].Account != "broadcaster" || got[0].Reason != "expired" {
		t.Errorf("entry = %+v, want broadcaster/expired", got[0])
	}
}

func TestTokenStatuses_HealthyReportsExpiryForEveryIdentity(t *testing.T) {
	withReauthConf(t, "tripbot4000", "adanalife_")
	botExp := time.Now().Add(3 * time.Hour)
	bcastExp := time.Now().Add(2 * time.Hour)
	cl := clientWithTokens(
		oauthtokens.Token{AccessToken: "good", ExpiresAt: botExp},
		oauthtokens.Token{AccessToken: "good", ExpiresAt: bcastExp},
	)

	got := cl.TokenStatuses()
	if len(got) != 2 {
		t.Fatalf("got %d statuses, want 2 (bot + broadcaster): %+v", len(got), got)
	}
	// Unlike AccountsNeedingReauth, healthy identities are still reported (with
	// their expiry) so the panel can show a countdown.
	if got[0].Account != "bot" || got[0].Reason != "" || !got[0].ExpiresAt.Equal(botExp) {
		t.Errorf("bot status = %+v, want healthy with botExp", got[0])
	}
	if got[1].Account != "broadcaster" || got[1].Reason != "" || !got[1].ExpiresAt.Equal(bcastExp) {
		t.Errorf("broadcaster status = %+v, want healthy with bcastExp", got[1])
	}
}

func TestTokenStatuses_CarriesReauthReason(t *testing.T) {
	withReauthConf(t, "tripbot4000", "adanalife_")
	healthy := oauthtokens.Token{AccessToken: "good", ExpiresAt: time.Now().Add(time.Hour)}
	cl := clientWithTokens(oauthtokens.Token{}, healthy) // bot blank → missing

	got := cl.TokenStatuses()
	if len(got) != 2 || got[0].Account != "bot" || got[0].Reason != "missing" {
		t.Fatalf("got %+v, want bot row with Reason=missing", got)
	}
	if !strings.Contains(got[0].InitURL, "account=bot") {
		t.Errorf("InitURL %q should target the bot account", got[0].InitURL)
	}
}

// When the bot and broadcaster are the same account, there's no separate
// broadcaster row — a blank broadcaster slot must not produce a phantom
// re-auth prompt.
func TestAccountsNeedingReauth_NoSeparateBroadcaster(t *testing.T) {
	withReauthConf(t, "tripbot4000", "tripbot4000")
	healthy := oauthtokens.Token{AccessToken: "good", ExpiresAt: time.Now().Add(time.Hour)}
	cl := clientWithTokens(healthy, oauthtokens.Token{}) // broadcaster slot blank, but irrelevant

	if got := cl.AccountsNeedingReauth(); got != nil {
		t.Fatalf("AccountsNeedingReauth() = %+v, want nil when no distinct broadcaster identity", got)
	}
}
