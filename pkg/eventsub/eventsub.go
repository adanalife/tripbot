// Package eventsub bootstraps the Twitch EventSub WebSocket client and
// dispatches typed events to caller-provided handlers.
//
// EventSub creates Twitch-side subscriptions authorized against the
// broadcaster identity; this package consumes the broadcaster's
// user-access-token + Twitch user ID and wraps
// github.com/joeyak/go-twitch-eventsub/v3.
//
// Lifecycle: Run blocks until the context is cancelled or the WebSocket
// session terminates fatally. The library handles session_reconnect
// frames transparently, but an outright socket close returns an error —
// so Run is one connection attempt, and the caller owns redialing it
// (cmd/tripbot runs it in a goroutine that loops with a delay).
//
// ErrUnauthorized is the exception the caller must honor: it means Twitch
// rejected the broadcaster token for every subscription, which no amount of
// redialing fixes, so the loop stops instead of spinning.
//
// Subscriptions are created in the OnWelcome callback (per Twitch's
// protocol — you can't subscribe until you have a session ID). If a
// subscribe call fails the error is logged and Run continues; partial
// subscription state is more useful than none.
package eventsub

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"

	twitch "github.com/joeyak/go-twitch-eventsub/v3"
)

// ErrUnauthorized reports that Twitch rejected the broadcaster token when
// creating subscriptions. Callers must not redial on it: the token is read once
// at startup, so retrying re-runs the same rejection forever (Twitch drops a
// subscription-less session with close code 4003 after ~10s, so a retry loop
// spins at that period). Recovery is a re-consent plus a restart.
var ErrUnauthorized = errors.New("eventsub: broadcaster token unauthorized")

// The status line Twitch returns for a rejected token, as the library renders it
// into the error message ("401 Unauthorized").
var unauthorizedStatus = strconv.Itoa(http.StatusUnauthorized) + " " + http.StatusText(http.StatusUnauthorized)

// Handlers carries the per-event callbacks the caller wants registered.
// All fields optional — leave a callback nil to skip subscribing to
// that event entirely (no Twitch-side subscription is created either).
type Handlers struct {
	OnFollow      func(username string)
	OnSubscribe   func(username string, isGift bool, tier string)
	OnUnsubscribe func(username string, isGift bool, tier string)
	// OnGift fires on channel.subscription.gift — the gifter's event, carrying
	// how many subs they gave (count) and whether the gift was anonymous (in
	// which case gifter is empty). Distinct from the per-recipient
	// channel.subscribe events (is_gift=true) that also fire.
	OnGift func(gifter string, count int, tier string, isAnonymous bool)
	// OnResub fires on channel.subscription.message — a resubscription the
	// viewer chose to share, carrying their cumulative + streak month counts
	// and the message they typed. channel.subscribe does not fire for resubs,
	// so this is the only signal for a continued subscription.
	OnResub func(username string, cumulativeMonths, streakMonths int, tier, message string)
}

// Config is the static input Run needs to subscribe. ClientID matches
// the Twitch app's client ID; BroadcasterToken is the broadcaster's
// user-access-token (no "oauth:" prefix); BroadcasterUserID is the
// numeric Twitch user ID — used for both broadcaster_user_id and
// moderator_user_id conditions on channel.follow v2.
type Config struct {
	ClientID          string
	BroadcasterToken  string
	BroadcasterUserID string
}

// Run dials the EventSub WebSocket, subscribes to the events for which
// Handlers has non-nil callbacks, and blocks until ctx is cancelled or
// the connection terminates. Returns nil on graceful ctx-driven
// shutdown; an error on connection failure or fatal protocol error.
func Run(ctx context.Context, cfg Config, h Handlers) error {
	if cfg.ClientID == "" || cfg.BroadcasterToken == "" || cfg.BroadcasterUserID == "" {
		return errors.New("eventsub: Config requires ClientID, BroadcasterToken, and BroadcasterUserID")
	}

	client := twitch.NewClient()

	client.OnError(func(err error) {
		slog.ErrorContext(ctx, "eventsub client error", "err", err)
	})

	client.OnRevoke(func(msg twitch.RevokeMessage) {
		slog.WarnContext(ctx, "eventsub subscription revoked by twitch — re-bootstrap broadcaster",
			"type", msg.Payload.Subscription.Type, "status", msg.Payload.Subscription.Status)
	})

	// Log every notification we receive, regardless of whether a typed
	// handler above is registered for it. Cheap observability: shows in Loki
	// what's actually firing in prod (and, by its absence, what isn't) so the
	// per-event chat-shout treatment can be designed from real data later. The
	// raw event JSON goes in the body, not a label — `type` is the only
	// (low-cardinality) key worth filtering on.
	client.OnRawEvent(func(event string, metadata twitch.MessageMetadata, subscription twitch.PayloadSubscription) {
		slog.InfoContext(ctx, "eventsub event",
			"type", string(subscription.Type),
			"message_id", metadata.MessageID,
			"payload", event,
		)
	})

	if h.OnFollow != nil {
		client.OnEventChannelFollow(func(e twitch.EventChannelFollow) {
			h.OnFollow(e.UserName)
		})
	}
	if h.OnSubscribe != nil {
		client.OnEventChannelSubscribe(func(e twitch.EventChannelSubscribe) {
			h.OnSubscribe(e.UserName, e.IsGift, e.Tier)
		})
	}
	if h.OnUnsubscribe != nil {
		client.OnEventChannelSubscriptionEnd(func(e twitch.EventChannelSubscriptionEnd) {
			h.OnUnsubscribe(e.UserName, e.IsGift, e.Tier)
		})
	}
	if h.OnGift != nil {
		client.OnEventChannelSubscriptionGift(func(e twitch.EventChannelSubscriptionGift) {
			h.OnGift(e.UserName, e.Total, e.Tier, e.IsAnonymous)
		})
	}
	if h.OnResub != nil {
		client.OnEventChannelSubscriptionMessage(func(e twitch.EventChannelSubscriptionMessage) {
			h.OnResub(e.UserName, e.CumulativeMonths, e.StreakMonths, e.Tier, e.Message.Text)
		})
	}

	// Written from the OnWelcome callback, which the library runs on its own read
	// goroutine, and read after ConnectWithContext returns — hence atomic.
	var attempted, denied atomic.Int32

	client.OnWelcome(func(msg twitch.WelcomeMessage) {
		sid := msg.Payload.Session.ID
		slog.InfoContext(ctx, "eventsub welcome received; subscribing", "session_id", sid)

		sub := func(ev twitch.EventSubscription, cond map[string]string) {
			attempted.Add(1)
			if subscribe(ctx, cfg, sid, ev, cond) {
				denied.Add(1)
			}
		}

		if h.OnFollow != nil {
			sub(twitch.SubChannelFollow, map[string]string{
				// channel.follow v2 requires both — moderator is the
				// identity reading the follow data; broadcaster is the
				// channel being followed. Same user in our case.
				"broadcaster_user_id": cfg.BroadcasterUserID,
				"moderator_user_id":   cfg.BroadcasterUserID,
			})
		}
		if h.OnSubscribe != nil {
			sub(twitch.SubChannelSubscribe, map[string]string{
				"broadcaster_user_id": cfg.BroadcasterUserID,
			})
		}
		if h.OnUnsubscribe != nil {
			sub(twitch.SubChannelSubscriptionEnd, map[string]string{
				"broadcaster_user_id": cfg.BroadcasterUserID,
			})
		}
		if h.OnGift != nil {
			sub(twitch.SubChannelSubscriptionGift, map[string]string{
				"broadcaster_user_id": cfg.BroadcasterUserID,
			})
		}
		if h.OnResub != nil {
			sub(twitch.SubChannelSubscriptionMessage, map[string]string{
				"broadcaster_user_id": cfg.BroadcasterUserID,
			})
		}
	})

	err := client.ConnectWithContext(ctx)
	// A wholly rejected token outranks whatever closed the socket: Twitch hangs
	// up on a subscription-less session (close code 4003), so the connection
	// error here is a symptom and redialing would just repeat it.
	if tokenRejected(attempted.Load(), denied.Load()) {
		return fmt.Errorf("%w (connection ended: %v)", ErrUnauthorized, err)
	}
	return err
}

// tokenRejected reports whether a subscribe round failed entirely on auth.
// Requiring *every* attempt to have been denied keeps the partial-subscription
// behavior this package documents: a token missing one event type's scope still
// gets the others, and the caller keeps redialing so a later socket drop
// recovers. Only a token that buys nothing is worth giving up on.
func tokenRejected(attempted, denied int32) bool {
	return attempted > 0 && denied == attempted
}

// subscribe creates a single Twitch-side subscription. A failure is logged but
// doesn't abort Run — losing one event type is better than losing all of them.
// Reports whether the failure was an expired or unscoped token, which is
// permanent until someone re-consents.
func subscribe(ctx context.Context, cfg Config, sessionID string, ev twitch.EventSubscription, cond map[string]string) (unauthorized bool) {
	_, err := twitch.SubscribeEventWithContext(ctx, twitch.SubscribeRequest{
		SessionID:   sessionID,
		ClientID:    cfg.ClientID,
		AccessToken: cfg.BroadcasterToken,
		Event:       ev,
		Condition:   cond,
	})
	if err != nil {
		slog.ErrorContext(ctx, "eventsub subscribe failed", "err", fmt.Errorf("event %s: %w", ev, err))
		return isUnauthorized(err)
	}
	slog.InfoContext(ctx, "eventsub subscribed", "event", string(ev))
	return false
}

// isUnauthorized reports whether Twitch rejected the token on a subscribe call.
// Matched on the message text because the library formats the HTTP status line
// into it (`could not subscribe to event: 401 Unauthorized: {...}`) rather than
// exposing a typed error or the status code.
func isUnauthorized(err error) bool {
	return err != nil && strings.Contains(err.Error(), unauthorizedStatus)
}
