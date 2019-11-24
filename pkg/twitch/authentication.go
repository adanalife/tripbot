package twitch

import (
	"log"

	"github.com/davecgh/go-spew/spew"
	"github.com/logrusorgru/aurora"
	"github.com/nicklaw5/helix"
)

// currentTwitchClient is the standard twitch client
var currentTwitchClient *helix.Client

// these are used to authenticate requests
var UserAccessToken string
var UserRefreshToken string

func FindOrCreateClient(clientID, clientSecret string) (*helix.Client, error) {
	// use the existing client if we have one
	if currentTwitchClient != nil {
		return currentTwitchClient, nil
	}
	client, err := helix.NewClient(&helix.Options{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		//TODO: maybe don't hardcode this
		// this is set at https://dev.twitch.tv/console/apps
		RedirectURI: "http://localhost:8080/auth/callback",
		//TODO: move to configs lib
		Scopes: []string{"openid", "user:edit:broadcast", "channel:read:subscriptions"},
	})
	currentTwitchClient = client
	return client, err
}

// code is returned after going through the OAuth flow
// it is set by the web server
func GenerateUserAccessToken(code string) {
	resp, err := currentTwitchClient.GetUserAccessToken(code)
	if err != nil {
		spew.Dump(err)
	}

	UserAccessToken = resp.Data.AccessToken
	UserRefreshToken = resp.Data.RefreshToken

	// update the current client with the access token
	currentTwitchClient.SetUserAccessToken(UserAccessToken)
}

func RefreshUserAccessToken() {
	// check to see if we have the required tokens to work with
	if UserRefreshToken == "" || UserAccessToken == "" {
		authURL := currentTwitchClient.GetAuthorizationURL("", false)
		log.Println("no user access token was present, did you log in with OAuth?")
		log.Println(aurora.Blue(authURL).Underline())
		return
	}

	resp, err := currentTwitchClient.RefreshUserAccessToken(UserRefreshToken)
	if err != nil {
		spew.Dump(err)
	}

	UserAccessToken = resp.Data.AccessToken
	UserRefreshToken = resp.Data.RefreshToken

	// update the current client with the new access token
	currentTwitchClient.SetUserAccessToken(UserAccessToken)

	spew.Dump(UserAccessToken, UserRefreshToken)
}
