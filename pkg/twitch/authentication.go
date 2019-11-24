package twitch

import (
	"log"

	"github.com/davecgh/go-spew/spew"
	"github.com/dmerrick/danalol-stream/pkg/helpers"
	"github.com/logrusorgru/aurora"
	"github.com/nicklaw5/helix"
)

// currentTwitchClient is the standard twitch client
var currentTwitchClient *helix.Client

// these are used to authenticate requests
var UserAccessToken string
var UserRefreshToken string

// Initialize creates a twitch client, or returns the existing one
func Initialize(clientID, clientSecret string) (*helix.Client, error) {
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

// GenerateUserAccessToken sends a code to Twitch to generate a
// user access token. This is called by the web server after
// going through the OAuth flow
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

// RefreshUserAccessToken makes a call to Twitch to generate a
// fresh user access token. It requires a UserRefreshToken to be
// set already.
func RefreshUserAccessToken() {
	// check to see if we have the required tokens to work with
	if UserRefreshToken == "" || UserAccessToken == "" {
		log.Println("no user access token was present, did you log in with OAuth?")
		authURL := currentTwitchClient.GetAuthorizationURL("", false)
		log.Println(aurora.Blue(authURL).Underline())
		// send a text message cause some features won't work
		// without a user access token
		helpers.SendSMS("refreshing user access token failed!")
		return
	}

	resp, err := currentTwitchClient.RefreshUserAccessToken(UserRefreshToken)
	if err != nil {
		spew.Dump(err)
		return
	}

	UserAccessToken = resp.Data.AccessToken
	UserRefreshToken = resp.Data.RefreshToken

	// update the current client with the new access token
	currentTwitchClient.SetUserAccessToken(UserAccessToken)

	log.Println(aurora.Cyan("successfully updated user access token"))
}
