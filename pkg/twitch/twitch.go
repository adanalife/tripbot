package twitch

import (
	"strings"

	"github.com/davecgh/go-spew/spew"
	"github.com/dmerrick/danalol-stream/pkg/config"
	terrors "github.com/dmerrick/danalol-stream/pkg/errors"
	"github.com/nicklaw5/helix"
)

// ChannelID contains the twitch-internal user ID
var ChannelID string

// subscribers is a list of the usernames of the current subscribers
//TODO: this could include tier and gift info if we wanted
var subscribers []string

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
		subscribers = append(subscribers, sub.UserName)
	}

	spew.Dump(subscribers)
}

func UserIsSubscriber(username string) bool {
	for _, sub := range subscribers {
		if username == sub {
			return true
		}
	}
	return false
}

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

	// get the twitch user_id
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
