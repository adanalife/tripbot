package twitch

import (
	"github.com/davecgh/go-spew/spew"
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
	// resp, err := currentTwitchUserClient.GetUserAccessToken(code)
	if err != nil {
		spew.Dump(err)
	}
	spew.Dump(resp)

	UserAccessToken = resp.Data.AccessToken
	UserRefreshToken = resp.Data.RefreshToken

	spew.Dump(UserAccessToken, UserRefreshToken)
}
