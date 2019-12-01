package main

import (
	"encoding/json"

	"github.com/davecgh/go-spew/spew"
	"github.com/nicklaw5/helix"
)

type SubscriptionWebhook struct {
	helix.ResponseCommon
	Data ManyEvents
}

type ManyEvents struct {
	Events []Event `json:"data"`
}

type Event struct {
	Id        string             `json:"id"`
	EventData helix.Subscription `json:"event_data"`
}

var err error

func main() {
	bodyBytes := []byte(`{"data":[{"event_data":{"broadcaster_id":"225469317","broadcaster_name":"ADanaLife_","is_gift":false,"plan_name":"Channel Subscription (adanalife_)","tier":"1000","user_id":"26784661","user_name":"MathGaming"},"event_timestamp":"2019-11-30T00:44:31Z","event_type":"subscriptions.subscribe","id":"1UJVQq8yMh9kOe0OmHpw3jbKkGH","version":"1.0"}]}`)

	resp3 := &SubscriptionWebhook{}
	err = json.Unmarshal(bodyBytes, &resp3.Data)
	spew.Dump(err)
	spew.Dump(resp3)
}
