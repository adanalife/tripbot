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

	"github.com/adanalife/tripbot/pkg/instrumentation"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// defaultTimeout bounds every gateway call. The gateway is an in-cluster
// neighbour, but it proxies the platform call synchronously and some platform
// writes are genuinely slow — facebook comment creation regularly takes
// 3.5–5s+, which a 5s bound turned into spurious context-canceled 502s for
// sends Graph had already accepted (a double-post hazard should sends ever
// retry). 15s covers the slow-write tail while still unwedging callers (a
// chat command, the watchdog tick, the chat-send path) from a hung gateway.
const defaultTimeout = 15 * time.Second

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
		instrumentation.GatewayConnection.Set(false)
		return fmt.Errorf("gateway send-chat: %w", err)
	}
	instrumentation.GatewayConnection.Set(true)
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

// Subscribers returns the channel's current subscribers as a login → tier map
// (GET /v1/subscribers). A login the gateway reports without a tier (an older
// gateway without the tiers field) defaults to tier 1.
func (c *Client) Subscribers(ctx context.Context) (map[string]int, error) {
	var body struct {
		Subscribers []string       `json:"subscribers"`
		Tiers       map[string]int `json:"tiers"`
	}
	if err := c.getJSON(ctx, "/v1/subscribers", &body); err != nil {
		return nil, err
	}
	subs := make(map[string]int, len(body.Subscribers))
	for _, login := range body.Subscribers {
		tier := body.Tiers[login]
		if tier < 1 {
			tier = 1
		}
		subs[login] = tier
	}
	return subs, nil
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

// InboundKind says what a viewer did. It mirrors the gateway's provider.InboundKind
// — duplicated by hand across the two repos, same convention as the eventbus
// envelopes; keep in sync.
type InboundKind string

const (
	// KindChat is a viewer comment. The zero value, so a page from a gateway
	// that predates the field decodes as comments.
	KindChat InboundKind = ""
	// KindGift is a viewer gift/donation: Text is empty and Gift is set.
	KindGift InboundKind = "gift"
)

// Gift is a viewer gift in platform-neutral shape.
//
// Diamonds is the per-gift value in the platform's creator-side unit (TikTok
// diamonds, roughly half the coin price the viewer paid) and Count is how many
// were sent in one action, so the gift's worth is Value(). Name is display
// text, not an identifier — platforms rename and retire gifts, so route on
// Value.
type Gift struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Count    int    `json:"count"`
	Diamonds int    `json:"diamonds"`
}

// Value is the gift's total worth in the platform's creator-side unit.
func (g Gift) Value() int { return g.Diamonds * g.Count }

// InboundChatMessage is one inbound live event — viewer activity only (the
// gateway filters the channel's own echoed sends). Kind says whether it's a
// comment or a gift; a consumer must switch on it rather than assume, because
// a gift's Text is empty and would parse as a blank command.
//
// Author is the human-facing name (a mutable display name on some platforms);
// AuthorID is the platform-native stable user ID — the key viewer persistence
// and identity linking must use.
type InboundChatMessage struct {
	Author   string      `json:"author"`
	AuthorID string      `json:"author_id"`
	Text     string      `json:"text"`
	Kind     InboundKind `json:"kind,omitempty"`
	Gift     *Gift       `json:"gift,omitempty"`
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

// Broadcast is the channel's current live broadcast (GET /v1/broadcast). VideoID
// is the watchable id (youtube.com/watch?v=<id>); Privacy is the visibility
// ("public"/"unlisted"/"private"). Live is false when no broadcast is active.
// BroadcastID and PermalinkURL are set by platforms whose broadcast object is
// distinct from the watchable video (facebook: the live-video id + the
// site-relative watch path, the only link that resolves an unpublished
// broadcast); empty elsewhere.
type Broadcast struct {
	VideoID      string `json:"video_id"`
	Live         bool   `json:"live"`
	Privacy      string `json:"privacy"`
	BroadcastID  string `json:"broadcast_id"`
	PermalinkURL string `json:"permalink_url"`
}

// ActiveBroadcast returns the channel's current live broadcast (GET
// /v1/broadcast). Only platforms with a discoverable broadcast object (YouTube)
// answer; a platform without one returns the gateway's 501 as an error.
func (c *Client) ActiveBroadcast(ctx context.Context) (Broadcast, error) {
	var b Broadcast
	if err := c.getJSON(ctx, "/v1/broadcast", &b); err != nil {
		return Broadcast{}, err
	}
	return b, nil
}

// StopEgress ends the platform's outbound broadcast (POST /v1/egress/stop).
// Only a gateway whose adapter manages the broadcast lifecycle (TikTok via
// Streamlabs) mounts the egress routes; anywhere else this 404s.
func (c *Client) StopEgress(ctx context.Context) error {
	return c.postEmpty(ctx, "/v1/egress/stop")
}

// StartEgress mints a fresh outbound broadcast (POST /v1/egress/start). No body
// is sent, which tells the gateway to resolve the title from its own metadata
// store rather than take one from the caller.
func (c *Client) StartEgress(ctx context.Context) error {
	return c.postEmpty(ctx, "/v1/egress/start")
}

// postEmpty POSTs to path with no body and discards a 2xx response.
func (c *Client) postEmpty(ctx context.Context, path string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("gateway %s request: %w", path, err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		instrumentation.GatewayConnection.Set(false)
		return fmt.Errorf("gateway %s: %w", path, err)
	}
	instrumentation.GatewayConnection.Set(true)
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("gateway %s: unexpected status %d", path, resp.StatusCode)
	}
	return nil
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
		instrumentation.GatewayConnection.Set(false)
		return nil, fmt.Errorf("gateway request: %w", err)
	}
	instrumentation.GatewayConnection.Set(true)
	return resp, nil
}
