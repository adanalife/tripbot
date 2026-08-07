package twitch

import (
	"sync"

	"github.com/adanalife/tripbot/pkg/oauthtokens"
)

// API owns the mutable Twitch state: the per-identity user-access-tokens and
// the cached channel/viewer/audience data. The platform-gateway is the single
// Helix caller, so nothing here talks to Twitch — the tokens are read from
// oauth_tokens (which the gateway keeps fresh) and the audience data is pushed
// in by the gateway refresh crons. Construct one with New().
//
// Methods are fronted by package-level free-function shims (see shims.go) that
// delegate to defaultClient; threading a constructed *API through callers and
// deleting the shims is a later step.
type API struct {
	// tokenMu guards currentUserToken (bot) and currentBroadcasterToken.
	// RWMutex because reads (BroadcasterUserAccessToken, TokenStatuses)
	// outnumber writes (LoadFromDB).
	tokenMu                 sync.RWMutex
	currentUserToken        oauthtokens.Token
	currentBroadcasterToken oauthtokens.Token

	// channelID is the twitch-internal user ID for the channel.
	channelID string

	// audienceMu guards subscribers, currentChatters, and chatterCount, which
	// are written by the gateway refresh crons and read from command dispatch
	// and the session-update cron. RWMutex because reads (UserIsSubscriber,
	// Chatters, ChatterCount) outnumber writes.
	audienceMu sync.RWMutex
	// subscribers maps each current subscriber's username to their
	// subscription tier (1–3).
	subscribers map[string]int
	// currentChatters holds the most recent chatter logins, cached from the gateway.
	currentChatters []string
	// chatterCount is the total reported by the API (may exceed
	// len(currentChatters) when the channel has more than one page of chatters).
	chatterCount int
}

// New constructs an API with zero mutable state. Tokens arrive via LoadFromDB
// and the audience caches via SetSubscribers / SetChatters.
func New() *API {
	return &API{}
}

// defaultClient backs the package-level free-function shims in shims.go. It
// preserves the previous "call twitch.Foo() from anywhere" surface while the
// globals are gone. Constructed at package-init, which reads no env and touches
// no network — importing this package has no side effects.
var defaultClient = New()
