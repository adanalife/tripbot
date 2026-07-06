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
// frames transparently; cmd/tripbot is expected to call Run in a
// goroutine and survive a Run-returns-error.
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

	twitch "github.com/joeyak/go-twitch-eventsub/v3"
)

// Handlers carries the per-event callbacks the caller wants registered.
// All fields optional — leave a callback nil to skip subscribing to
// that event entirely (no Twitch-side subscription is created either).
type Handlers struct {
	OnFollow      func(username string)
	OnSubscribe   func(username string, isGift bool, tier string)
	OnUnsubscribe func(username string, isGift bool, tier string)
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

	client.OnWelcome(func(msg twitch.WelcomeMessage) {
		sid := msg.Payload.Session.ID
		slog.InfoContext(ctx, "eventsub welcome received; subscribing", "session_id", sid)

		if h.OnFollow != nil {
			subscribe(ctx, cfg, sid, twitch.SubChannelFollow, map[string]string{
				// channel.follow v2 requires both — moderator is the
				// identity reading the follow data; broadcaster is the
				// channel being followed. Same user in our case.
				"broadcaster_user_id": cfg.BroadcasterUserID,
				"moderator_user_id":   cfg.BroadcasterUserID,
			})
		}
		if h.OnSubscribe != nil {
			subscribe(ctx, cfg, sid, twitch.SubChannelSubscribe, map[string]string{
				"broadcaster_user_id": cfg.BroadcasterUserID,
			})
		}
		if h.OnUnsubscribe != nil {
			subscribe(ctx, cfg, sid, twitch.SubChannelSubscriptionEnd, map[string]string{
				"broadcaster_user_id": cfg.BroadcasterUserID,
			})
		}
	})

	return client.ConnectWithContext(ctx)
}

// subscribe creates a single Twitch-side subscription. Errors are
// logged but don't abort Run — losing one event type is better than
// losing all of them.
func subscribe(ctx context.Context, cfg Config, sessionID string, ev twitch.EventSubscription, cond map[string]string) {
	_, err := twitch.SubscribeEventWithContext(ctx, twitch.SubscribeRequest{
		SessionID:   sessionID,
		ClientID:    cfg.ClientID,
		AccessToken: cfg.BroadcasterToken,
		Event:       ev,
		Condition:   cond,
	})
	if err != nil {
		slog.ErrorContext(ctx, "eventsub subscribe failed", "err", fmt.Errorf("event %s: %w", ev, err))
		return
	}
	slog.InfoContext(ctx, "eventsub subscribed", "event", string(ev))
}
