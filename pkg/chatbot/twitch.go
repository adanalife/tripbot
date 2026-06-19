package chatbot

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/feature"
	mytwitch "github.com/adanalife/tripbot/pkg/twitch"
)

// twitchGatewayFlagKey is the runtime kill-switch for routing the chatbot's
// Helix calls through the platform-gateway. It defaults off (the flag row
// doesn't exist until toggled), so even an env wired with TWITCH_API_URL stays
// in-process until the flag is flipped on — the cutover (and instant revert,
// no restart) is a console toggle. Only meaningful when TWITCH_API_URL is set.
const twitchGatewayFlagKey = "chatbot.twitch_gateway"

// Twitch is the command-time Twitch Helix surface the chatbot needs. Today
// that's only follow lookups (followageCmd); it grows as admin-panel Helix
// features land (ban/timeout, send-as-broadcaster).
//
// It is deliberately the seam where, if the Twitch Helix API ever moves into
// its own service, the in-process adapter below is swapped for an HTTP
// client — without touching any command code.
type Twitch interface {
	FollowedAt(username string) (time.Time, bool)
}

// newTwitch wires the production Twitch adapter. With no TWITCH_API_URL there's
// no gateway to reach, so it's the plain in-process path (zero-config default).
// When a gateway IS wired, it returns a flaggedTwitch that dispatches per call
// based on the twitchGatewayFlagKey runtime flag — so the gateway is wired but
// dormant until flipped on, and revertible without a restart.
func newTwitch(a *App) Twitch {
	if c.Conf.TwitchAPIURL == "" {
		return realTwitch{}
	}
	return flaggedTwitch{
		app:     a,
		gateway: newGatewayTwitch(c.Conf.TwitchAPIURL),
		inproc:  realTwitch{},
	}
}

// flaggedTwitch routes each call to the gateway or the in-process adapter based
// on the live twitchGatewayFlagKey value, read from the App's flag client at
// call time (cmd/tripbot reassigns a.Flags to the Postgres-backed, console-
// toggleable client after New(), so the flag flips without a bot restart).
type flaggedTwitch struct {
	app     *App
	gateway Twitch
	inproc  Twitch
}

func (f flaggedTwitch) FollowedAt(username string) (time.Time, bool) {
	if f.app.Flags.Bool(context.Background(), twitchGatewayFlagKey, feature.EvalContext{}) {
		return f.gateway.FollowedAt(username)
	}
	return f.inproc.FollowedAt(username)
}

// realTwitch is the in-process adapter — used when no gateway URL is
// configured. Delegates to the package-level pkg/twitch shim (defaultClient).
// Mirrors the realVLC / realOnscreens shape.
type realTwitch struct{}

func (realTwitch) FollowedAt(username string) (time.Time, bool) {
	return mytwitch.FollowedAt(username)
}

// gatewayTwitch is the HTTP adapter — it reaches the platform-gateway
// gateway-twitch instance instead of calling Helix in-process. It satisfies the
// same Twitch interface, so command code is untouched (the payoff of the
// #738/#739 injection seam).
type gatewayTwitch struct {
	baseURL string
	client  *http.Client
}

func newGatewayTwitch(baseURL string) gatewayTwitch {
	return gatewayTwitch{
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{Timeout: 5 * time.Second},
	}
}

// FollowedAt asks the gateway when username followed the channel
// (GET /v1/followed-at/{login}). It fails closed (ok=false) on any transport,
// decode, or non-2xx error — matching the in-process adapter, so gateway
// trouble degrades a follow check rather than blocking the command. A 404 is
// the gateway's "not a follower" answer and is expected, not logged.
func (g gatewayTwitch) FollowedAt(username string) (time.Time, bool) {
	endpoint := g.baseURL + "/v1/followed-at/" + url.PathEscape(username)
	resp, err := g.client.Get(endpoint)
	if err != nil {
		slog.Warn("gateway FollowedAt request failed", "username", username, "err", err)
		return time.Time{}, false
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		var body struct {
			FollowedAt time.Time `json:"followed_at"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			slog.Warn("gateway FollowedAt decode failed", "username", username, "err", err)
			return time.Time{}, false
		}
		return body.FollowedAt, true
	case http.StatusNotFound:
		return time.Time{}, false // not a follower — expected
	default:
		slog.Warn("gateway FollowedAt non-2xx", "username", username, "status", resp.StatusCode)
		return time.Time{}, false
	}
}
