package twitch

import (
	"net/http"
	"strconv"

	"github.com/adanalife/tripbot/pkg/instrumentation"
)

// rateLimitRecorder is a RoundTripper that reads Twitch Helix's
// Ratelimit-* response headers and surfaces them as OTel gauges, so
// dashboards / alerts can see remaining headroom without waiting for
// 429s. Helix returns these headers on every response; the per-bearer
// quota is shared across all calls from the same App Access Token.
type rateLimitRecorder struct{ next http.RoundTripper }

func (rt rateLimitRecorder) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := rt.next.RoundTrip(req)
	if err != nil || resp == nil {
		return resp, err
	}
	if v := resp.Header.Get("Ratelimit-Remaining"); v != "" {
		if n, perr := strconv.Atoi(v); perr == nil {
			instrumentation.TwitchHelixRateLimit.SetRemaining(int64(n))
		}
	}
	if v := resp.Header.Get("Ratelimit-Limit"); v != "" {
		if n, perr := strconv.Atoi(v); perr == nil {
			instrumentation.TwitchHelixRateLimit.SetLimit(int64(n))
		}
	}
	return resp, err
}
