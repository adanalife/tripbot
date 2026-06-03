package twitch

import (
	"context"
	"time"

	"github.com/nicklaw5/helix/v2"
)

// This file preserves the package-level free-function API that callers used
// before pkg/twitch held a *Client. Each shim delegates to defaultClient so
// existing call sites (pkg/users, pkg/server, pkg/obs/watchdog, cmd/tripbot,
// cmd/auth-bootstrap, pkg/eventsub, pkg/chatbot) keep working unchanged while
// the package-level mutable globals are gone.
//
// These shims are transitional. Once a constructed *Client is threaded through
// those callers, this whole file (and defaultClient) is deleted — see the
// Phase B/C plan and vault/decisions/chatbot-app-injection-pattern.md.

// --- auth / token ---

func Client() (*helix.Client, error)            { return defaultClient.Client() }
func BroadcasterClient() (*helix.Client, error) { return defaultClient.BroadcasterClient() }
func LoadFromDB() error                         { return defaultClient.LoadFromDB() }
func IRCAuthToken() string                      { return defaultClient.IRCAuthToken() }
func CurrentUserAccessToken() string            { return defaultClient.CurrentUserAccessToken() }
func BroadcasterUserAccessToken() string        { return defaultClient.BroadcasterUserAccessToken() }
func AccountsNeedingReauth() []AccountReauth    { return defaultClient.AccountsNeedingReauth() }
func TokenStatuses() []AccountTokenStatus       { return defaultClient.TokenStatuses() }

func GenerateUserAccessToken(code string, expectedLogin string) error {
	return defaultClient.GenerateUserAccessToken(code, expectedLogin)
}
func RefreshUserAccessToken(ctx context.Context) { defaultClient.RefreshUserAccessToken(ctx) }
func Reauth(ctx context.Context, account string) { defaultClient.Reauth(ctx, account) }

// --- audience / viewer queries ---

func GetSubscribers(ctx context.Context)    { defaultClient.GetSubscribers(ctx) }
func GetFollowerCount(ctx context.Context)  { defaultClient.GetFollowerCount(ctx) }
func UserIsSubscriber(username string) bool { return defaultClient.UserIsSubscriber(username) }
func UserIsFollower(username string) bool   { return defaultClient.UserIsFollower(username) }
func FollowedAt(username string) (time.Time, bool) {
	return defaultClient.FollowedAt(username)
}
func ChatterCount() int             { return defaultClient.ChatterCount() }
func Chatters() map[string]struct{} { return defaultClient.Chatters() }
func UpdateChatters()               { defaultClient.UpdateChatters() }
func ChannelID() string             { return defaultClient.ChannelID() }

func SendChatMessageAsBroadcaster(ctx context.Context, text string) error {
	return defaultClient.SendChatMessageAsBroadcaster(ctx, text)
}

// --- streams ---

func IsChannelLive(ctx context.Context, login string) (bool, error) {
	return defaultClient.IsChannelLive(ctx, login)
}
