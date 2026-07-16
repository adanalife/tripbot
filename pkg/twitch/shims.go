package twitch

import (
	"github.com/nicklaw5/helix/v2"
)

// This file preserves the package-level free-function API that callers used
// before pkg/twitch held a *Client. Each shim delegates to defaultClient so
// existing call sites (pkg/users, pkg/server, cmd/tripbot, pkg/eventsub,
// pkg/chatbot) keep working unchanged while the package-level mutable globals
// are gone.
//
// These shims are transitional. Once a constructed *Client is threaded through
// those callers, this whole file (and defaultClient) is deleted.

// --- auth / token ---

func Client() (*helix.Client, error)      { return defaultClient.Client() }
func LoadFromDB() error                   { return defaultClient.LoadFromDB() }
func IRCAuthToken() string                { return defaultClient.IRCAuthToken() }
func BroadcasterUserAccessToken() string  { return defaultClient.BroadcasterUserAccessToken() }
func TokenStatuses() []AccountTokenStatus { return defaultClient.TokenStatuses() }

// --- cached audience state (fed from the platform-gateway) ---

func UserIsSubscriber(username string) bool  { return defaultClient.UserIsSubscriber(username) }
func UserSubscriberTier(username string) int { return defaultClient.UserSubscriberTier(username) }
func ChatterCount() int                      { return defaultClient.ChatterCount() }
func Chatters() map[string]struct{}          { return defaultClient.Chatters() }
func ChannelID() string                      { return defaultClient.ChannelID() }
func SetSubscribers(tiers map[string]int)    { defaultClient.SetSubscribers(tiers) }
func SetChatters(logins []string, count int) { defaultClient.SetChatters(logins, count) }
func SetChannelID(id string)                 { defaultClient.SetChannelID(id) }
