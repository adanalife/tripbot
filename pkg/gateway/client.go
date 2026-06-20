// Package gateway is tripbot's HTTP client for the platform-gateway
// (gateway-twitch / gateway-youtube) — the per-platform API service that owns
// the Helix / YouTube-Data-API call surface. tripbot reaches it instead of
// calling the platform API in-process, so the gateway can become the single
// token holder (the Secrets-Manager token-move prerequisite).
//
// The client is a thin, stateless request/response wrapper over the gateway's
// v1 JSON endpoints. It holds no platform-specific knowledge and triggers no
// init-time side effects, so any binary or package may import it (see the
// package-boundary-init-discipline ADR). Callers decide their own
// fail-open/fail-closed posture from the returned error.
package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// defaultTimeout bounds every gateway call. The gateway is an in-cluster
// neighbour, so a few seconds is generous; a hung gateway must not wedge a
// caller (a chat command, the watchdog tick, the chat-send path).
const defaultTimeout = 5 * time.Second

// Chat identities accepted by SendChat, matching the gateway's
// provider.Identity values. The empty string lets the gateway pick its default.
const (
	IdentityBot         = "bot"
	IdentityBroadcaster = "broadcaster"
)

// Client talks to one platform-gateway instance over its v1 JSON API.
type Client struct {
	baseURL string
	http    *http.Client
}

// New builds a Client for the gateway reachable at baseURL (e.g.
// http://gateway-twitch:8080). A trailing slash is tolerated.
func New(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: defaultTimeout},
	}
}

// BaseURL returns the normalised gateway base URL (trailing slash trimmed).
func (c *Client) BaseURL() string { return c.baseURL }

// FollowedAt asks when login followed the channel (GET /v1/followed-at/{login}).
// A returned ok=false with a nil error is the gateway's 404 "not a follower"
// answer — an expected result, not a failure. A non-nil error means the call
// itself failed (transport, decode, or upstream non-2xx); callers choose how to
// degrade.
func (c *Client) FollowedAt(ctx context.Context, login string) (time.Time, bool, error) {
	resp, err := c.get(ctx, "/v1/followed-at/"+url.PathEscape(login))
	if err != nil {
		return time.Time{}, false, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		var body struct {
			FollowedAt time.Time `json:"followed_at"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			return time.Time{}, false, fmt.Errorf("gateway followed-at decode: %w", err)
		}
		return body.FollowedAt, true, nil
	case http.StatusNotFound:
		return time.Time{}, false, nil // not a follower — expected
	default:
		return time.Time{}, false, fmt.Errorf("gateway followed-at: unexpected status %d", resp.StatusCode)
	}
}

// IsLive reports whether login is currently streaming (GET /v1/live/{login}).
func (c *Client) IsLive(ctx context.Context, login string) (bool, error) {
	resp, err := c.get(ctx, "/v1/live/"+url.PathEscape(login))
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("gateway live: unexpected status %d", resp.StatusCode)
	}
	var body struct {
		Live bool `json:"live"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return false, fmt.Errorf("gateway live decode: %w", err)
	}
	return body.Live, nil
}

// SendChat posts text to the channel's chat as identity ("bot" / "broadcaster";
// "" lets the gateway pick its default) via POST /v1/chat.
func (c *Client) SendChat(ctx context.Context, identity, text string) error {
	payload, err := json.Marshal(map[string]string{"identity": identity, "text": text})
	if err != nil {
		return fmt.Errorf("gateway send-chat encode: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/chat", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("gateway send-chat request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("gateway send-chat: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("gateway send-chat: unexpected status %d", resp.StatusCode)
	}
	return nil
}

// get issues a GET against the gateway, joining path onto the base URL.
func (c *Client) get(ctx context.Context, path string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("gateway request: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gateway request: %w", err)
	}
	return resp, nil
}
