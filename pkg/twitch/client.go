package twitch

import (
	"sync"

	"github.com/adanalife/tripbot/pkg/oauthtokens"
	"github.com/nicklaw5/helix/v2"
)

// API owns the mutable Twitch state that used to live in package-level
// globals: the two helix clients, the App Access Token, the per-identity
// user-access-tokens, and the cached channel/viewer/audience data. It is the
// in-process implementation of "talk to Twitch"; construct one with New().
//
// (Named API rather than Client because the package already exposes a
// Client() helix-getter that callers depend on — the type can be renamed once
// the shims in shims.go are deleted.)
//
// Fields are grouped into two clusters on purpose. The **auth core** (helix
// clients + tokens) is the cohesive unit that may eventually move into its
// own auth service — keeping it visually distinct here marks that future
// seam. The **query/viewer** cluster is cached read-state derived from Helix.
//
// Methods are still fronted by package-level free-function shims (see
// shims.go) delegating to defaultClient, so existing callers are unchanged
// while the globals are eliminated; threading a constructed *API through
// callers and deleting the shims is a later step.
type API struct {
	// --- auth core (future auth-service boundary) ---

	// currentTwitchClient is the lazy-initialized bot helix client, built by
	// Client() for the OAuth bootstrap's identity check (GetUsers) and the IRC
	// readiness probe. The Helix query surface now lives in the platform-gateway.
	currentTwitchClient *helix.Client
	// appAccessToken is set in Client() (Client Credentials grant).
	appAccessToken string

	// tokenMu guards currentUserToken (bot) and currentBroadcasterToken.
	// RWMutex because reads (IRCAuthToken, BroadcasterUserAccessToken,
	// TokenStatuses) outnumber writes (LoadFromDB).
	tokenMu                 sync.RWMutex
	currentUserToken        oauthtokens.Token
	currentBroadcasterToken oauthtokens.Token

	// --- query / viewer state (cached Helix reads) ---

	// channelID is the twitch-internal user ID for the channel.
	channelID string
	// subscribers is the usernames of the current subscribers.
	subscribers []string
	// currentChatters holds the most recent chatter list, cached from the gateway.
	currentChatters []helix.ChatChatter
	// chatterCount is the total reported by the API (may exceed
	// len(currentChatters) when the channel has more than one page of chatters).
	chatterCount int
}

// New constructs an API with zero mutable state. The helix client and App
// Access Token are built lazily on first use (Client()); the static
// ClientID/ClientSecret credentials are read from env in init().
func New() *API {
	return &API{}
}

// defaultClient backs the package-level free-function shims in shims.go. It
// preserves the previous "call twitch.Foo() from anywhere" surface while the
// globals are gone. Constructed at package-init; New() touches no env, so it
// is safe before init() populates ClientID/ClientSecret.
var defaultClient = New()
