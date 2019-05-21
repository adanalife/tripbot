package twitch

import (
	"log"

	"github.com/dmerrick/danalol-stream/pkg/config"
	"github.com/nicklaw5/helix"
)

var currentTwitchClient *helix.Client

func FindOrCreateClient(clientID string) (*helix.Client, error) {
	// use the existing client if we have one
	if currentTwitchClient != nil {
		return currentTwitchClient, nil
	}
	client, err := helix.NewClient(&helix.Options{
		ClientID: clientID,
	})
	currentTwitchClient = client
	return client, err
}

func UserIsFollower(user string) bool {
	// I can't follow myself so just do this
	if user == config.ChannelName {
		return true
	}

	//TODO a better way to do this?
	client, err := FindOrCreateClient("")
	if err != nil {
		return false
	}

	usersResp, err := client.GetUsers(&helix.UsersParams{
		Logins: []string{
			user,
		},
	})
	if err != nil {
		log.Println("error getting user info", err)
		return false
	}

	// get the twitch user_id
	userID := usersResp.Data.Users[0].ID

	followsResp, err := client.GetUsersFollows(&helix.UsersFollowsParams{
		ToID:   config.ChannelID,
		FromID: userID,
	})
	if err != nil {
		log.Println("error getting user follows", err)
		return false
	}

	if followsResp.Data.Total < 1 {
		return false
	}
	return true

}
