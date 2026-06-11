package youtube

import (
	"context"

	ytapi "google.golang.org/api/youtube/v3"
)

// Package-level free-function API delegating to a default Client, matching
// pkg/twitch's transitional shim pattern: callers (pkg/server's auth
// handlers, cmd/tripbot's boot sequence) use these until a constructed
// *Client is threaded through, at which point this file is deleted — see
// pkg/twitch/shims.go and vault/decisions/chatbot-app-injection-pattern.md.

var defaultClient = New()

func LoadFromDB() error                                   { return defaultClient.LoadFromDB() }
func ChannelID() string                                   { return defaultClient.ChannelID() }
func Service(ctx context.Context) (*ytapi.Service, error) { return defaultClient.Service(ctx) }
func AuthCodeURL(state string) string                     { return defaultClient.AuthCodeURL(state) }

func GenerateUserAccessToken(ctx context.Context, code string) (string, error) {
	return defaultClient.GenerateUserAccessToken(ctx, code)
}
