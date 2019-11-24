package twitch

import (
	"log"
	"strings"

	"github.com/dmerrick/danalol-stream/pkg/config"
	terrors "github.com/dmerrick/danalol-stream/pkg/errors"
	"github.com/nicklaw5/helix"
)

// ChannelID contains the twitch-internal user ID
var ChannelID string

// subscribers is a list of the usernames of the current subscribers
//TODO: this could include tier and gift info if we wanted
var subscribers []string

//TODO: turn this into a helper and use it in UserIsFollower() as well
// getChannelID makes a request to twitch to get the user ID for the channel
func getChannelID() {
	resp, err := currentTwitchClient.GetUsers(&helix.UsersParams{
		Logins: []string{config.ChannelName},
	})
	if err != nil {
		terrors.Log(err, "error getting user info from twitch")
	}
	ChannelID = resp.Data.Users[0].ID
}

// GetSubscribers pulls down the most recent list of subscribers
func GetSubscribers() {
	if ChannelID == "" {
		getChannelID()
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
func UserIsFollower(user string) bool {
	// I can't follow myself so just do this
	if user == strings.ToLower(config.ChannelName) {
		return true
	}

	client := currentTwitchClient

	usersResp, err := client.GetUsers(&helix.UsersParams{
		Logins: []string{
			user,
		},
	})
	if err != nil {
		terrors.Log(err, "error getting user info")
		return false
	}

	// pull out the twitch user_id
	userID := usersResp.Data.Users[0].ID

	followsResp, err := client.GetUsersFollows(&helix.UsersFollowsParams{
		ToID:   config.ChannelID,
		FromID: userID,
	})
	if err != nil {
		terrors.Log(err, "error getting user follows")
		return false
	}

	if followsResp.Data.Total < 1 {
		return false
	}
	return true

}
