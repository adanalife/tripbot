package twitch

import (
	"github.com/davecgh/go-spew/spew"
	terrors "github.com/dmerrick/danalol-stream/pkg/errors"
	"github.com/nicklaw5/helix"
)

// currentTwitchClient is the standard twitch client
var currentTwitchClient *helix.Client

// the user client is used for user-authenticated calls
// (set tags, get subs, etc.)
//TODO: does this really need to be separate?
var currentTwitchUserClient *helix.Client

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

func InitializeUserClient(clientID, clientSecret string) (*helix.Client, error) {
	// use the existing client if we have one
	if currentTwitchUserClient != nil {
		return currentTwitchUserClient, nil
	}

	// we didn't have one, so we create a new one
	client, err := helix.NewClient(&helix.Options{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		//TODO: maybe don't hardcode this
		RedirectURI: "http://localhost:8080/auth/callback",
	})
	if err != nil {
		terrors.Log(err, "error creating user client")
		return client, err
	}

	currentTwitchUserClient = client
	return client, err
}

// code is returned after going through the OAuth flow
// it is set by the web server
func GenerateUserAccessToken(code string) {
	//if UserAccessCode == "" {
	//	log.Println(aurora.Red("no UserAccessCode, you must go through the OAauth flow"))
	//	//TODO: generate the URL here? open the window?
	//	return
	//}

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
