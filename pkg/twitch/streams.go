package twitch

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/nicklaw5/helix/v2"
)

// IsChannelLive returns true when the given Twitch login currently has an
// active stream per Helix GetStreams. Used by the OBS silent-disconnect
// watchdog to detect the half-open RTMP state where OBS thinks it's
// streaming but Twitch has dropped the broadcast.
//
// Authorizes against the app-access-token — GetStreams is public and does
// not need a user token.
func (cl *API) IsChannelLive(ctx context.Context, login string) (bool, error) {
	if login == "" {
		return false, fmt.Errorf("IsChannelLive: empty login")
	}
	client, err := cl.Client()
	if err != nil {
		return false, fmt.Errorf("twitch API client unavailable: %w", err)
	}
	resp, err := client.GetStreams(&helix.StreamsParams{
		UserLogins: []string{login},
	})
	if err != nil {
		return false, fmt.Errorf("GetStreams: %w", err)
	}
	if cl.checkHelixResp(ctx, "GetStreams", "", &resp.ResponseCommon) {
		return false, fmt.Errorf("GetStreams returned %d", resp.StatusCode)
	}
	live := len(resp.Data.Streams) > 0
	slog.DebugContext(ctx, "twitch live check", "login", login, "live", live)
	return live, nil
}
