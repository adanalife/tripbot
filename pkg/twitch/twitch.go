package twitch

import (
	"context"
	"fmt"
	"log"
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
// ctx is forward-compat plumbing — the helix client doesn't accept ctx
// yet, so the Helix HTTP call isn't currently linked under the parent
// cron span. Threading it now lets future ctx-aware helix wrappers nest
// automatically.
func GetSubscribers(_ context.Context) {
	//TODO: should we do this elsewhere as well?
	if ChannelID == "" {
		ChannelID = getChannelID(c.Conf.ChannelName)
	}
	resp, err := currentTwitchClient.GetSubscriptions(&helix.SubscriptionsParams{
		BroadcasterID: ChannelID,
	})
	if err != nil {
		terrors.Log(err, "error getting subscriptions from twitch")
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
		log.Println("subscribers:", strings.Join(subscribers, ", "))
	} else {
		log.Println(c.Conf.ChannelName, "has no subscribers :(")
	}
}

// GetFollowerCount fetches the current total follower count for the
// channel. ctx is forward-compat plumbing (see GetSubscribers).
func GetFollowerCount(_ context.Context) {
	if ChannelID == "" {
		ChannelID = getChannelID(c.Conf.ChannelName)
	}
	resp, err := currentTwitchClient.GetChannelFollows(&helix.GetChannelFollowsParams{
		BroadcasterID: ChannelID,
	})
	if err != nil {
		terrors.Log(err, "error getting follower count from twitch")
		return
	}
	if checkHelixResp("GetChannelFollows", &resp.ResponseCommon) {
		return
	}
	instrumentation.TwitchAudience.SetFollowers(int64(resp.Data.Total))
	log.Printf("%s has %d followers", c.Conf.ChannelName, resp.Data.Total)
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
