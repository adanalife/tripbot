package server

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"

	"github.com/davecgh/go-spew/spew"
	terrors "github.com/dmerrick/tripbot/pkg/errors"
	mytwitch "github.com/dmerrick/tripbot/pkg/twitch"
	"github.com/nicklaw5/helix"
)

// TwitchAuthentication is sensitive internal twitch access tokens,
// which are made available via the server to supporting scripts
// (if properly authenticated)
type TwitchAuthentication struct {
	ChannelID       string `json:"channel_id"`
	UserAccessToken string `json:"user_access_token"`
	ClientID        string `json:"client_id"`
	AppAccessToken  string `json:"app_access_token"`
}

// twitchAuthJSON returns the Twitch access tokens as JSON
func twitchAuthJSON() string {
	var jsonData []byte
	auth := TwitchAuthentication{
		ChannelID:       mytwitch.ChannelID,
		UserAccessToken: mytwitch.UserAccessToken,
		ClientID:        mytwitch.ClientID,
		AppAccessToken:  mytwitch.AppAccessToken,
	}
	jsonData, err := json.Marshal(auth)
	if err != nil {
		terrors.Log(err, "unable to marshal twitch auth")
	}
	return string(jsonData)
}

// example payload:
// {"data":[{"event_data":{"broadcaster_id":"225469317","broadcaster_name":"ADanaLife_","is_gift":false,"plan_name":"Channel Subscription (adanalife_)","tier":"1000", "user_id":"26784661","user_name":"MathGaming"},"event_timestamp":"2019-11-30T00:44:31Z","event_type":"subscriptions.subscribe","id":"1UJVQq8yMh9kOe0OmHpw3jbKkGH","version":"1.0"}]}
type SubscriptionWebhook struct {
	helix.ResponseCommon
	Data ManyEvents
}

type ManyEvents struct {
	Events []Event `json:"data"`
}

type Event struct {
	Id           string             `json:"id"`
	Subscription helix.Subscription `json:"event_data"`
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

func decodeSubscriptionWebhookResponse(r *http.Request) (*SubscriptionWebhook, error) {
	log.Println("decoding subscription webhook")

	// we use a custom struct because the 3rd party lib doesnt support webhooks yet
	resp := &SubscriptionWebhook{}
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
