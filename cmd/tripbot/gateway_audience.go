package main

import (
	"context"
	"log/slog"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/instrumentation"
	mytwitch "github.com/adanalife/tripbot/pkg/twitch"
)

// gatewayChatterSource is the cmd-wired users.ChatterSource. Chatter refresh and
// the live follower check route through the platform-gateway (the single Helix
// caller); the cached reads (Chatters / ChatterCount / IsSubscriber) read
// tripbot's in-process audience cache — which the refresh populates from the
// gateway — so they need no dispatch.
type gatewayChatterSource struct{ t *Tripbot }

func (s gatewayChatterSource) Chatters() map[string]struct{} { return mytwitch.Chatters() }
func (s gatewayChatterSource) ChatterCount() int             { return mytwitch.ChatterCount() }

func (s gatewayChatterSource) IsSubscriber(username string) bool {
	return mytwitch.UserIsSubscriber(username)
}

// UpdateChatters refreshes the cached chatter set from the gateway. A gateway
// error keeps the prior cached set, matching the don't-zero-on-error posture.
func (s gatewayChatterSource) UpdateChatters() {
	ctx := context.Background()
	count, logins, err := s.t.gateway.Chatters(ctx)
	if err != nil {
		slog.WarnContext(ctx, "gateway chatters refresh failed; keeping cached set", "err", err)
		return
	}
	mytwitch.SetChatters(logins, count)
}

// IsFollower reports whether username follows the channel, via the gateway. The
// broadcaster can't follow themselves, so admins short-circuit to true; a
// gateway error fails closed (treated as non-follower).
func (s gatewayChatterSource) IsFollower(username string) bool {
	ctx := context.Background()
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

// refreshSubscribers refreshes the cached subscriber list from the gateway.
// Driven at startup and by the 5-minute cron. A gateway error keeps the prior
// cached list.
func (t *Tripbot) refreshSubscribers(ctx context.Context) {
	subs, err := t.gateway.Subscribers(ctx)
	if err != nil {
		slog.WarnContext(ctx, "gateway subscribers refresh failed; keeping cached list", "err", err)
		return
	}
	mytwitch.SetSubscribers(subs)
}

// refreshFollowerCount refreshes the follower-count gauge from the gateway.
func (t *Tripbot) refreshFollowerCount(ctx context.Context) {
	total, err := t.gateway.Followers(ctx)
	if err != nil {
		slog.WarnContext(ctx, "gateway follower-count refresh failed", "err", err)
		return
	}
	instrumentation.TwitchAudience.SetFollowers(int64(total))
}
