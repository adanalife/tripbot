package chatbot

import (
	"context"
	"log/slog"
	"time"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/gateway"
)

// Twitch is the command-time Twitch Helix surface the chatbot needs. Today
// that's only follow lookups (followageCmd); it grows as admin-panel Helix
// features land (ban/timeout, send-as-broadcaster).
//
// The Helix API is owned by the platform-gateway (gateway-twitch); this seam is
// the HTTP client that reaches it, so command code stays untouched (the payoff
// of the #738/#739 injection seam).
type Twitch interface {
	FollowedAt(username string) (time.Time, bool)
}

// newTwitch wires the production Twitch adapter — the platform-gateway HTTP
// client, the single Helix caller since the cutover. A non-Twitch instance
// (PLATFORM=youtube) has no TWITCH_API_URL and thus no Twitch Helix surface, so
// it gets a fail-closed no-op adapter.
func newTwitch(*App) Twitch {
	if c.Conf.TwitchAPIURL == "" {
		return noTwitch{}
	}
	return newGatewayTwitch(c.Conf.TwitchAPIURL)
}

// noTwitch is the fail-closed adapter for instances with no gateway wired (a
// non-Twitch bot). Every lookup reports "unknown", matching the gateway
// adapter's fail-closed posture.
type noTwitch struct{}

func (noTwitch) FollowedAt(string) (time.Time, bool) { return time.Time{}, false }

// gatewayTwitch is the HTTP adapter — it reaches the platform-gateway
// gateway-twitch instance (via the shared pkg/gateway client) instead of
// calling Helix in-process. It satisfies the same Twitch interface, so command
// code is untouched (the payoff of the #738/#739 injection seam).
type gatewayTwitch struct {
	client *gateway.Client
}

func newGatewayTwitch(baseURL string) gatewayTwitch {
	return gatewayTwitch{client: gateway.New(baseURL)}
}

// FollowedAt asks the gateway when username followed the channel. It fails
// closed (ok=false) on any transport, decode, or non-2xx error, so gateway
// trouble degrades a follow check rather than blocking the command. The
// gateway's "not a follower" answer (a clean 404) comes back as ok=false with
// no error and is not logged.
func (g gatewayTwitch) FollowedAt(username string) (time.Time, bool) {
	when, ok, err := g.client.FollowedAt(context.Background(), username)
	if err != nil {
		slog.Warn("gateway FollowedAt failed", "username", username, "err", err)
		return time.Time{}, false
	}
	return when, ok
}
