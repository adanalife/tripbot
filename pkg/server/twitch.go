package server

import (
	"encoding/json"

	terrors "github.com/dmerrick/danalol-stream/pkg/errors"
	mytwitch "github.com/dmerrick/danalol-stream/pkg/twitch"
)

type TwitchAuthentication struct {
	ChannelID       string `json:"channel_id"`
	UserAccessToken string `json:"user_access_token"`
	ClientID        string `json:"client_id"`
	AppAccessToken  string `json:"app_access_token"`
	//TODO: do we need these ever?
	// AuthToken       string `json:"auth_token"`
	// ClientSecret    string `json:"client_secret"`
}

func TwitchAuthJSON() string {
	var jsonData []byte
	auth := TwitchAuthentication{
		ChannelID:       mytwitch.ChannelID,
		UserAccessToken: mytwitch.UserAccessToken,
		ClientID:        mytwitch.ClientID,
		AppAccessToken:  mytwitch.AppAccessToken,
		//TODO: do we need these ever?
		// AuthToken:       mytwitch.AuthToken,
		// ClientSecret:    mytwitch.ClientSecret,
	}
	jsonData, err := json.Marshal(auth)
	if err != nil {
		terrors.Log(err, "unable to marshal twitch auth")
	}
	return string(jsonData)
}
