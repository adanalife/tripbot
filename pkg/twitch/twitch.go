package twitch

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	terrors "github.com/adanalife/tripbot/pkg/errors"
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
	resp, err := currentTwitchClient.GetUsers(&helix.UsersParams{
		Logins: []string{username},
	})
	if err != nil {
		terrors.Log(err, "error getting user info from twitch")
		return ""
	}
	if resp == nil {
		terrors.Log(nil, "empty response from twitch")
		return ""
	}
	if checkHelixResp("GetUsers", &resp.ResponseCommon) {
		return ""
	}

	if len(resp.Data.Users) < 1 {
		terrors.Log(fmt.Errorf("missing data"), "no user in response from twitch")
		return ""
	}
	return resp.Data.Users[0].ID
}

// GetSubscribers pulls down the most recent list of subscribers.
// ctx is forward-compat plumbing for nesting helix HTTP under the parent
// cron span; the helix client doesn't accept ctx yet, but the log lines
// get a trace_id link via slog.InfoContext.
func GetSubscribers(ctx context.Context) {
	//TODO: should we do this elsewhere as well?
	if ChannelID == "" {
		ChannelID = getChannelID(c.Conf.ChannelName)
	}
	resp, err := currentTwitchClient.GetSubscriptions(&helix.SubscriptionsParams{
		BroadcasterID: ChannelID,
	})
	if err != nil {
		terrors.LogContext(ctx, err, "error getting subscriptions from twitch")
		return
	}
	if checkHelixResp("GetSubscriptions", &resp.ResponseCommon) {
		// keep the prior subscriber list rather than zeroing it out
		return
	}

	// spew.Dump(resp)

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
// channel. ctx is forward-compat plumbing (see GetSubscribers).
func GetFollowerCount(ctx context.Context) {
	if ChannelID == "" {
		ChannelID = getChannelID(c.Conf.ChannelName)
	}
	resp, err := currentTwitchClient.GetChannelFollows(&helix.GetChannelFollowsParams{
		BroadcasterID: ChannelID,
	})
	if err != nil {
		terrors.LogContext(ctx, err, "error getting follower count from twitch")
		return
	}
	if checkHelixResp("GetChannelFollows", &resp.ResponseCommon) {
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

// UserIsFollower returns true if the user follows the channel
func UserIsFollower(username string) bool {
	// I can't follow myself so just do this
	if c.UserIsAdmin(username) {
		return true
	}

	// get the channel ID for the given user
	userID := getChannelID(username)

	resp, err := currentTwitchClient.GetChannelFollows(&helix.GetChannelFollowsParams{
		BroadcasterID: ChannelID,
		UserID:        userID,
	})
	if err != nil {
		terrors.Log(err, "error getting user follows")
		return false
	}
	if checkHelixResp("GetChannelFollows", &resp.ResponseCommon) {
		// fail closed: when we can't verify follow status, treat as non-follower
		return false
	}

	if resp.Data.Total < 1 {
		return false
	}
	return true

}
