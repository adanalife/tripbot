package main

import (
	// "fmt"
	// "log"

	"log"
	"os"

	"github.com/davecgh/go-spew/spew"
	"github.com/nicklaw5/helix"
)

const (
	ourChannelID = "225469317"
)

func UserIsFollower(user string) (bool, error) {
	// first we must check for required ENV vars
	twitchClientID, ok := os.LookupEnv("TWITCH_CLIENT_ID")
	if !ok {
		panic("You must set TWITCH_CLIENT_ID")
	}

	client, err := helix.NewClient(&helix.Options{
		ClientID: twitchClientID,
	})
	if err != nil {
		log.Fatal("error creating twitch client", err)
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

	usersToCheck := []string{
		"bleo",
		"pokimane",
		"tripbot4000",
		"mathgaming",
		"shroud",
		"olivecat50",
		"sithdaddy",
	}

	for _, user := range usersToCheck {
		following, err := UserIsFollower(user)
		if err != nil {
			log.Fatal(err)
		}

		spew.Dump(user, following)
	}
}
