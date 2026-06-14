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

	// currentTwitchClient is the lazy-initialized bot helix client. IRC auth +
	// any Helix endpoint authorized against the bot's identity goes through it.
	currentTwitchClient *helix.Client
	// broadcasterTwitchClient is the lazy-initialized broadcaster helix client.
	// Endpoints that authorize against the channel-owner identity
	// (GetSubscriptions, GetChannelFollows total, channel.update, mod actions)
	// go through it — the bot client would 401 since the user-access-token's
	// identity matters, not just its scope set.
	broadcasterTwitchClient *helix.Client
	// appAccessToken is set in Client() (Client Credentials grant) and shared
	// by both helix clients.
	appAccessToken string

	// tokenMu guards currentUserToken (bot) and currentBroadcasterToken.
	// RWMutex because reads (IRCAuthToken, CurrentUserAccessToken) outnumber
	// writes (LoadFromDB, refresh).
	tokenMu                 sync.RWMutex
	currentUserToken        oauthtokens.Token
	currentBroadcasterToken oauthtokens.Token

	// --- query / viewer state (cached Helix reads) ---

	// channelID is the twitch-internal user ID for the channel.
	channelID string
	// botID is the Twitch user ID for the bot account (moderator identity for
	// API calls that require moderator:read:chatters).
	botID string
	// subscribers is the usernames of the current subscribers.
	subscribers []string
	// currentChatters holds the most recent chatter list from the Helix API.
	currentChatters []helix.ChatChatter
	// chatterCount is the total reported by the API (may exceed
	// len(currentChatters) when the channel has more than one page of chatters).
	chatterCount int
}

// New constructs an API with zero mutable state. The helix clients and App
// Access Token are built lazily on first use (Client()/BroadcasterClient());
// the static ClientID/ClientSecret credentials are read from env in init().
func New() *API {
	return &API{}
}

// defaultClient backs the package-level free-function shims in shims.go. It
// preserves the previous "call twitch.Foo() from anywhere" surface while the
// globals are gone. Constructed at package-init; New() touches no env, so it
// is safe before init() populates ClientID/ClientSecret.
var defaultClient = New()
