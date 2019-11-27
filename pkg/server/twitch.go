package server

import (
	"encoding/json"

	terrors "github.com/dmerrick/danalol-stream/pkg/errors"
	mytwitch "github.com/dmerrick/danalol-stream/pkg/twitch"
)

type TwitchAuthentication struct {
	ChannelID       string `json:"channel_id"`
	UserAccessToken string `json:"user_access_token"`
}

func TwitchAuthJSON() string {
	var jsonData []byte
	auth := TwitchAuthentication{
		ChannelID:       mytwitch.ChannelID,
		UserAccessToken: mytwitch.UserAccessToken,
	}
	jsonData, err := json.Marshal(auth)
	if err != nil {
		terrors.Log(err, "unable to marshal twitch auth")
	}
	return string(jsonData)
}
