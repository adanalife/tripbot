package twitch

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/instrumentation"
	"github.com/nicklaw5/helix/v2"
)

// ChannelID contains the twitch-internal user ID
var ChannelID string

// subscribers is a list of the usernames of the current subscribers
//TODO: this could include tier and gift info if we wanted
var subscribers []string

// getChannelID makes a request to twitch to get the user ID for the channel
func getChannelID(username string) string {
	client, err := Client()
	if err != nil {
		slog.Error("twitch API client unavailable", "err", err)
		return ""
	}
	resp, err := client.GetUsers(&helix.UsersParams{
		Logins: []string{username},
	})
	if err != nil {
		slog.Error("error getting user info from twitch", "err", err)
		return ""
	}
	if resp == nil {
		slog.Error("empty response from twitch")
		return ""
	}
	// account="" — GetUsers here authorizes against the app-access-token, not a
	// user token, so re-reading a user token wouldn't fix a 401.
	if checkHelixResp(context.Background(), "GetUsers", "", &resp.ResponseCommon) {
		return ""
	}

	if len(resp.Data.Users) < 1 {
		slog.Error("no user in response from twitch", "err", fmt.Errorf("missing data"))
		return ""
	}
	return resp.Data.Users[0].ID
}

// GetSubscribers pulls down the most recent list of subscribers.
// Authorizes against the broadcaster identity — the bot's user-access-token
// can't read the channel owner's subs no matter what scopes it has.
// ctx is forward-compat plumbing for nesting helix HTTP under the parent
// cron span; the helix client doesn't accept ctx yet, but the log lines
// get a trace_id link via slog.InfoContext.
func GetSubscribers(ctx context.Context) {
	if !broadcasterTokenLoaded() {
		slog.InfoContext(ctx, "skipping GetSubscribers: no broadcaster oauth_tokens row")
		return
	}
	bclient, err := BroadcasterClient()
	if err != nil {
		slog.ErrorContext(ctx, "broadcaster helix client unavailable", "err", err)
		return
	}
	//TODO: should we do this elsewhere as well?
	if ChannelID == "" {
		ChannelID = getChannelID(c.Conf.ChannelName)
	}
	resp, err := bclient.GetSubscriptions(&helix.SubscriptionsParams{
		BroadcasterID: ChannelID,
	})
	if err != nil {
		slog.ErrorContext(ctx, "error getting subscriptions from twitch", "err", err)
		return
	}
	if checkHelixResp(ctx, "GetSubscriptions", "broadcaster", &resp.ResponseCommon) {
		// keep the prior subscriber list rather than zeroing it out
		return
	}

	// reset the current subscriber list
	subscribers = []string{}

	// pull out the usernames
	for _, sub := range resp.Data.Subscriptions {
		subscribers = append(subscribers, strings.ToLower(sub.UserName))
	}

	instrumentation.TwitchAudience.SetSubscribers(int64(len(subscribers)))

	if len(subscribers) > 0 {
		slog.InfoContext(ctx, "subscribers", "count", len(subscribers), "names", strings.Join(subscribers, ", "))
	} else {
		slog.InfoContext(ctx, "no subscribers", "channel", c.Conf.ChannelName)
	}
}

// GetFollowerCount fetches the current total follower count for the
// channel. Authorizes against the broadcaster identity (moderator:read:followers
// on the channel-owner token). ctx is forward-compat plumbing (see GetSubscribers).
func GetFollowerCount(ctx context.Context) {
	if !broadcasterTokenLoaded() {
		slog.InfoContext(ctx, "skipping GetFollowerCount: no broadcaster oauth_tokens row")
		return
	}
	bclient, err := BroadcasterClient()
	if err != nil {
		slog.ErrorContext(ctx, "broadcaster helix client unavailable", "err", err)
		return
	}
	if ChannelID == "" {
		ChannelID = getChannelID(c.Conf.ChannelName)
	}
	resp, err := bclient.GetChannelFollows(&helix.GetChannelFollowsParams{
		BroadcasterID: ChannelID,
	})
	if err != nil {
		slog.ErrorContext(ctx, "error getting follower count from twitch", "err", err)
		return
	}
	if checkHelixResp(ctx, "GetChannelFollows", "broadcaster", &resp.ResponseCommon) {
		return
	}
	instrumentation.TwitchAudience.SetFollowers(int64(resp.Data.Total))
	slog.InfoContext(ctx, "follower count", "channel", c.Conf.ChannelName, "total", resp.Data.Total)
}

// UserIsSubscriber returns true if the user subscribes to the channel
func UserIsSubscriber(username string) bool {
	for _, sub := range subscribers {
		if username == sub {
			return true
		}
	}
	return false
}

// UserIsFollower returns true if the user follows the channel.
// Authorizes against the broadcaster identity (moderator:read:followers).
// When the broadcaster token isn't loaded yet, fail closed.
func UserIsFollower(username string) bool {
	// I can't follow myself so just do this
	if c.UserIsAdmin(username) {
		return true
	}

	if !broadcasterTokenLoaded() {
		return false
	}
	bclient, err := BroadcasterClient()
	if err != nil {
		slog.Error("broadcaster helix client unavailable", "err", err)
		return false
	}

	// get the channel ID for the given user
	userID := getChannelID(username)

	resp, err := bclient.GetChannelFollows(&helix.GetChannelFollowsParams{
		BroadcasterID: ChannelID,
		UserID:        userID,
	})
	if err != nil {
		slog.Error("error getting user follows", "err", err)
		return false
	}
	if checkHelixResp(context.Background(), "GetChannelFollows", "broadcaster", &resp.ResponseCommon) {
		// fail closed: when we can't verify follow status, treat as non-follower
		return false
	}

	if resp.Data.Total < 1 {
		return false
	}
	return true

}

// FollowedAt returns when username started following the channel, and whether
// they follow at all. Like UserIsFollower, it authorizes against the
// broadcaster identity (moderator:read:followers) and fails closed: ok=false
// when the broadcaster token isn't loaded, the lookup errors, or the user
// doesn't follow.
func FollowedAt(username string) (time.Time, bool) {
	if !broadcasterTokenLoaded() {
		return time.Time{}, false
	}
	bclient, err := BroadcasterClient()
	if err != nil {
		slog.Error("broadcaster helix client unavailable", "err", err)
		return time.Time{}, false
	}

	userID := getChannelID(username)

	resp, err := bclient.GetChannelFollows(&helix.GetChannelFollowsParams{
		BroadcasterID: ChannelID,
		UserID:        userID,
	})
	if err != nil {
		slog.Error("error getting user follows", "err", err)
		return time.Time{}, false
	}
	if checkHelixResp("GetChannelFollows", &resp.ResponseCommon) {
		return time.Time{}, false
	}

	if resp.Data.Total < 1 || len(resp.Data.Channels) < 1 {
		return time.Time{}, false
	}
	return resp.Data.Channels[0].Followed.Time, true
}

// broadcasterTokenLoaded reports whether the broadcaster's user-access-token
// has been populated. Cheaper than building the helix client just to discover
// the user-token slot is empty.
func broadcasterTokenLoaded() bool {
	tokenMu.RLock()
	defer tokenMu.RUnlock()
	return currentBroadcasterToken.AccessToken != ""
}
