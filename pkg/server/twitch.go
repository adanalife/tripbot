package server

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"

	"github.com/davecgh/go-spew/spew"
	terrors "github.com/dmerrick/danalol-stream/pkg/errors"
	mytwitch "github.com/dmerrick/danalol-stream/pkg/twitch"
	"github.com/nicklaw5/helix"
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

func decodeFollowWebhookResponse(r *http.Request) (*helix.UsersFollowsResponse, error) {
	log.Println("decoding user webhook")
	// resp := &helix.Response{}

	resp := &helix.UsersFollowsResponse{}
	bodyBytes, err := ioutil.ReadAll(r.Body)
	if err != nil {
		terrors.Log(err, "failed to read request body")
		return resp, err
	}

	// print the webhook contents
	// fmt.Println(string(bodyBytes) + "\n")

	// Only attempt to decode the response if we have a response we can handle
	if len(bodyBytes) > 0 && resp.StatusCode < http.StatusInternalServerError {
		if resp.StatusCode < http.StatusBadRequest {
			// if resp.Data != nil && resp.StatusCode < http.StatusBadRequest {
			// Successful request
			err = json.Unmarshal(bodyBytes, &resp.Data)
		} else {
			// Failed request
			err = json.Unmarshal(bodyBytes, &resp)
		}

		if err != nil {
			terrors.Log(err, "failed to decode API response")
		}
	}
	return resp, err
}

func decodeSubscriptionWebhookResponse(r *http.Request) (*helix.SubscriptionsResponse, error) {
	log.Println("decoding subscription webhook")

	resp := &helix.SubscriptionsResponse{}
	bodyBytes, err := ioutil.ReadAll(r.Body)
	if err != nil {
		terrors.Log(err, "failed to read request body")
		return resp, err
	}

	// print the webhook contents
	fmt.Println(string(bodyBytes) + "\n")

	// Only attempt to decode the response if we have a response we can handle
	if len(bodyBytes) > 0 && resp.StatusCode < http.StatusInternalServerError {
		if resp.StatusCode < http.StatusBadRequest {
			// if resp.Data != nil && resp.StatusCode < http.StatusBadRequest {
			// Successful request
			err = json.Unmarshal(bodyBytes, &resp.Data)
		} else {
			// Failed request
			err = json.Unmarshal(bodyBytes, &resp)
		}

		if err != nil {
			terrors.Log(err, "failed to decode API response")
		}
	}

	spew.Dump(resp.Data)
	return resp, err
}
