package main

import (
	"context"
	"log/slog"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/instrumentation"
	mytwitch "github.com/adanalife/tripbot/pkg/twitch"
)

// gatewayChatterSource is the cmd-wired users.ChatterSource for instances that
// may use the platform-gateway. Chatter refresh and the live follower check
// route through the gateway when useGateway reports the runtime flag is on, else
// in-process. The cached reads (Chatters / ChatterCount / IsSubscriber) always
// read tripbot's in-process audience cache — which the refresh populates from
// whichever source is active — so they need no dispatch.
//
// With no gateway wired (t.gateway == nil) useGateway is always false, so this
// behaves exactly like the plain in-process Twitch source; that's why it can be
// the source unconditionally.
type gatewayChatterSource struct{ t *Tripbot }

func (s gatewayChatterSource) Chatters() map[string]struct{} { return mytwitch.Chatters() }
func (s gatewayChatterSource) ChatterCount() int             { return mytwitch.ChatterCount() }

func (s gatewayChatterSource) IsSubscriber(username string) bool {
	return mytwitch.UserIsSubscriber(username)
}

// UpdateChatters refreshes the cached chatter set, from the gateway when the
// flag is on, else the in-process Helix poll. A gateway error keeps the prior
// cached set, matching UpdateChatters' don't-zero-on-error posture.
func (s gatewayChatterSource) UpdateChatters() {
	ctx := context.Background()
	if !s.t.useGateway(ctx) {
		mytwitch.UpdateChatters()
		return
	}
	count, logins, err := s.t.gateway.Chatters(ctx)
	if err != nil {
		slog.WarnContext(ctx, "gateway chatters refresh failed; keeping cached set", "err", err)
		return
	}
	mytwitch.SetChatters(logins, count)
}

// IsFollower reports whether username follows the channel, via the gateway when
// the flag is on, else in-process. Mirrors UserIsFollower's admin short-circuit
// (the broadcaster can't follow themselves) and its fail-closed-on-error posture.
func (s gatewayChatterSource) IsFollower(username string) bool {
	ctx := context.Background()
	if !s.t.useGateway(ctx) {
		return mytwitch.UserIsFollower(username)
	}
	if c.UserIsAdmin(username) {
		return true
	}
	_, ok, err := s.t.gateway.FollowedAt(ctx, username)
	if err != nil {
		slog.WarnContext(ctx, "gateway follower check failed; treating as non-follower",
			"username", username, "err", err)
		return false
	}
	return ok
}

// refreshSubscribers refreshes the cached subscriber list — from the gateway
// when the flag is on, else the in-process Helix poll. Driven at startup and by
// the 5-minute cron. A gateway error keeps the prior cached list.
func (t *Tripbot) refreshSubscribers(ctx context.Context) {
	if !t.useGateway(ctx) {
		mytwitch.GetSubscribers(ctx)
		return
	}
	subs, err := t.gateway.Subscribers(ctx)
	if err != nil {
		slog.WarnContext(ctx, "gateway subscribers refresh failed; keeping cached list", "err", err)
		return
	}
	mytwitch.SetSubscribers(subs)
}

// refreshFollowerCount refreshes the follower-count gauge — from the gateway
// when the flag is on, else the in-process Helix poll.
func (t *Tripbot) refreshFollowerCount(ctx context.Context) {
	if !t.useGateway(ctx) {
		mytwitch.GetFollowerCount(ctx)
		return
	}
	total, err := t.gateway.Followers(ctx)
	if err != nil {
		slog.WarnContext(ctx, "gateway follower-count refresh failed", "err", err)
		return
	}
	instrumentation.TwitchAudience.SetFollowers(int64(total))
}
