package twitch

import (
	"log"

	"github.com/davecgh/go-spew/spew"
	"github.com/adanalife/tripbot/pkg/config"
	terrors "github.com/adanalife/tripbot/pkg/errors"
	"github.com/nicklaw5/helix"
)

var followsTopic = []string{
	"https://api.twitch.tv/helix/users/follows?first=1&to_id=" + ChannelID,
	"/webhooks/twitch/users/follows",
}
var subsTopic = []string{
	"https://api.twitch.tv/helix/subscriptions/events?first=1&broadcaster_id=" + ChannelID,
	"/webhooks/twitch/subscriptions/events",
}

// UpdateWebhookSubscriptions will create new webhook subscriptions
func UpdateWebhookSubscriptions() {
	subscribeToWebhook(followsTopic)
	// since the staging account isn't an affiliate, don't bother
	if config.IsProduction() {
		subscribeToWebhook(subsTopic)
	}
	getWebookSubscriptions()
}

func subscribeToWebhook(pair []string) {
	topic := pair[0]
	endpoint := pair[1]

	//TODO: eventually use Secret param too
	_, err := currentTwitchClient.PostWebhookSubscription(&helix.WebhookSubscriptionPayload{
		Mode:         "subscribe",
		Topic:        topic,
		Callback:     config.ExternalURL + endpoint,
		LeaseSeconds: 24 * 60 * 60, // 24h is the max allowed
	})

	if err != nil {
		terrors.Log(err, "failed to create webhook subscription for "+endpoint)
	}
}

func getWebookSubscriptions() {
	// talk to twitch and see what the current webhooks are
	//TODO: this doesnt seem to work, like at all
	resp, err := currentTwitchClient.GetWebhookSubscriptions(&helix.WebhookSubscriptionsParams{})
	if err != nil {
		terrors.Log(err, "failed to get webhook subscriptions")
	}

	if resp.Data.Total > 0 {
		if config.Verbose {
			spew.Dump(resp.Data.WebhookSubscriptions)
		}
	} else {
		log.Println("no webhooks found")
	}
}
