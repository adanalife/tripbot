package twitch

import (
	"strings"

	"github.com/dmerrick/danalol-stream/pkg/config"
	terrors "github.com/dmerrick/danalol-stream/pkg/errors"
	"github.com/nicklaw5/helix"
)

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
