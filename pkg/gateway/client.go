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

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
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
//
// The transport is otelhttp-wrapped so every call starts a client span and
// injects the W3C traceparent header (via the global propagator that
// telemetry.Init installs). The gateway's otelhttp handler extracts it, making
// its server span a child of this one — so a chat command and the Helix call it
// triggers form a single cross-service trace. With tracing disabled the
// propagator is a no-op, so this is inert.
func New(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		http: &http.Client{
			Timeout:   defaultTimeout,
			Transport: otelhttp.NewTransport(http.DefaultTransport),
		},
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

// UserID resolves login to the platform's internal user/channel ID
// (GET /v1/users/{login}). It's the gateway-routed replacement for the
// in-process getChannelID side effect that EventSub's BroadcasterUserID needs —
// once Helix calls route through the gateway, nothing else populates the ID.
func (c *Client) UserID(ctx context.Context, login string) (string, error) {
	var body struct {
		ID string `json:"id"`
	}
	if err := c.getJSON(ctx, "/v1/users/"+url.PathEscape(login), &body); err != nil {
		return "", err
	}
	if body.ID == "" {
		return "", fmt.Errorf("gateway users/%s: empty id", login)
	}
	return body.ID, nil
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

// Chatters returns the channel's current chatter logins and the authoritative
// total (GET /v1/chatters). The total can exceed len(logins) when the channel
// has more chatters than the API returns in one page.
func (c *Client) Chatters(ctx context.Context) (count int, logins []string, err error) {
	var body struct {
		Count    int      `json:"count"`
		Chatters []string `json:"chatters"`
	}
	if err := c.getJSON(ctx, "/v1/chatters", &body); err != nil {
		return 0, nil, err
	}
	return body.Count, body.Chatters, nil
}

// Subscribers returns the channel's current subscriber logins
// (GET /v1/subscribers).
func (c *Client) Subscribers(ctx context.Context) ([]string, error) {
	var body struct {
		Subscribers []string `json:"subscribers"`
	}
	if err := c.getJSON(ctx, "/v1/subscribers", &body); err != nil {
		return nil, err
	}
	return body.Subscribers, nil
}

// Followers returns the channel's total follower count (GET /v1/followers).
func (c *Client) Followers(ctx context.Context) (int, error) {
	var body struct {
		Total int `json:"total"`
	}
	if err := c.getJSON(ctx, "/v1/followers", &body); err != nil {
		return 0, err
	}
	return body.Total, nil
}

// InboundChatMessage is one inbound live-chat line — viewer messages only (the
// gateway filters the channel's own echoed sends).
type InboundChatMessage struct {
	Author string `json:"author"`
	Text   string `json:"text"`
}

// InboundChatPage is one page from GET /v1/chat/inbound. Cursor is opaque: pass
// it back verbatim on the next call ("" to start, or after the gateway reports
// offline). Live is false when no broadcast is active. PollAfterMS is the
// gateway's suggested wait before the next poll (live cadence, rediscover wait,
// or quota backoff) — the caller's only pacing input.
type InboundChatPage struct {
	Messages    []InboundChatMessage `json:"messages"`
	Cursor      string               `json:"cursor"`
	Live        bool                 `json:"live"`
	PollAfterMS int                  `json:"poll_after_ms"`
}

// InboundChat fetches a page of inbound live chat, advancing the opaque cursor
// (GET /v1/chat/inbound). The gateway owns discovery, paging, backlog-skip,
// own-echo filtering, and pacing, so the caller just forwards the cursor. Only
// platforms whose inbound chat is polled (YouTube) answer; a streaming-chat
// platform returns the gateway's 501 as an error.
func (c *Client) InboundChat(ctx context.Context, cursor string) (InboundChatPage, error) {
	path := "/v1/chat/inbound"
	if cursor != "" {
		path += "?cursor=" + url.QueryEscape(cursor)
	}
	var page InboundChatPage
	if err := c.getJSON(ctx, path, &page); err != nil {
		return InboundChatPage{}, err
	}
	return page, nil
}

// getJSON issues a GET and decodes a 200 JSON body into dest; any non-200 or
// decode failure is returned as an error.
func (c *Client) getJSON(ctx context.Context, path string, dest any) error {
	resp, err := c.get(ctx, path)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("gateway %s: unexpected status %d", path, resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(dest); err != nil {
		return fmt.Errorf("gateway %s decode: %w", path, err)
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
