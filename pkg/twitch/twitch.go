package twitch

import (
	"fmt"
	"log"
	"strings"

	"github.com/adanalife/tripbot/pkg/config"
	terrors "github.com/adanalife/tripbot/pkg/errors"
	"github.com/adanalife/tripbot/pkg/helpers"
	"github.com/nicklaw5/helix"
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
	}
	if len(resp.Data.Users) < 1 {
		terrors.Log(fmt.Errorf("missing data"), "no user in response from twitch")
		return ""
	}
	return resp.Data.Users[0].ID
}

// GetSubscribers pulls down the most recent list of subscribers
func GetSubscribers() {
	//TODO: should we do this elsewhere as well?
	if ChannelID == "" {
		ChannelID = getChannelID(config.ChannelName)
	}
	resp, err := currentTwitchClient.GetSubscriptions(&helix.SubscriptionsParams{
		BroadcasterID: ChannelID,
	})
	if err != nil {
		terrors.Log(err, "error getting subscriptions from twitch")
	}

	// reset the current subscriber list
	subscribers = []string{}

	// pull out the usernames
	for _, sub := range resp.Data.Subscriptions {
		subscribers = append(subscribers, strings.ToLower(sub.UserName))
	}

	if len(subscribers) > 0 {
		log.Println("subscribers:", strings.Join(subscribers, ", "))
	} else {
		log.Println(config.ChannelName, "has no subscribers :(")
	}
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
	if helpers.UserIsAdmin(username) {
		return true
	}

	// get the channel ID for the given user
	userID := getChannelID(username)

	resp, err := currentTwitchClient.GetUsersFollows(&helix.UsersFollowsParams{
		ToID:   ChannelID,
		FromID: userID,
	})
	if err != nil {
		terrors.Log(err, "error getting user follows")
		return false
	}

	if resp.Data.Total < 1 {
		return false
	}
	return true

}
