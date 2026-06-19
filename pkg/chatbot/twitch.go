package chatbot

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	mytwitch "github.com/adanalife/tripbot/pkg/twitch"
)

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

// newTwitch picks the production Twitch adapter: the platform-gateway HTTP
// client when TWITCH_API_URL is set, otherwise the in-process pkg/twitch path.
// This is the Phase 3 flip — selecting by config keeps the in-process path as
// the zero-config default, so envs that haven't been pointed at a gateway are
// unchanged.
func newTwitch() Twitch {
	if c.Conf.TwitchAPIURL != "" {
		return newGatewayTwitch(c.Conf.TwitchAPIURL)
	}
	return realTwitch{}
}

// realTwitch is the in-process adapter — used when no gateway URL is
// configured. Delegates to the package-level pkg/twitch shim (defaultClient).
// Mirrors the realVLC / realOnscreens shape.
type realTwitch struct{}

func (realTwitch) FollowedAt(username string) (time.Time, bool) {
	return mytwitch.FollowedAt(username)
}

// gatewayTwitch is the HTTP adapter — it reaches the platform-gateway
// twitch-api instance instead of calling Helix in-process. It satisfies the
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
