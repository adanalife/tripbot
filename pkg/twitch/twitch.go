package twitch

import (
	// "fmt"
	// "log"

	"log"

	"github.com/davecgh/go-spew/spew"
	"github.com/nicklaw5/helix"
)

const (
	ourChannelID = "225469317"
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

func UserIsFollower(user string) (bool, error) {
	//TODO a better way to do this?
	client, err := FindOrCreateClient("")
	if err != nil {
		return false, err
	}

	usersResp, err := client.GetUsers(&helix.UsersParams{
		Logins: []string{
			user,
		},
	})
	if err != nil {
		log.Println("error getting user info", err)
		return false, err
	}

	// get the twitch user_id
	userID := usersResp.Data.Users[0].ID

	followsResp, err := client.GetUsersFollows(&helix.UsersFollowsParams{
		ToID:   ourChannelID,
		FromID: userID,
	})
	if err != nil {
		log.Println("error getting user follows", err)
		return false, err
	}

	if followsResp.Data.Total < 1 {
		return false, err
	}
	return true, err

}

func main() {

	// 3z9codsl6ke8np8y2o2xhintw9visz
	// deopjfdsrzg7bgltu7sf2iu7zqoao8

	usersToCheck := []string{
		"pokimane",
		"bleo",
		"tripbot4000",
		"mathgaming",
		"shroud",
	}

	for _, user := range usersToCheck {
		following, err := UserIsFollower(user)
		if err != nil {
			log.Fatal(err)
		}

		spew.Dump(user, following)
	}
}
