package twitch

import (
	"log"

	"github.com/davecgh/go-spew/spew"
	"github.com/dmerrick/danalol-stream/pkg/config"
	terrors "github.com/dmerrick/danalol-stream/pkg/errors"
	"github.com/nicklaw5/helix"
)

// UpdateWebhookSubscriptions will create new webhook subscriptions
//TODO: use the twitch API instead of a shell script when possible
func UpdateWebhookSubscriptions() {

	// c.p. https://dev.twitch.tv/docs/api/webhooks-reference
	topics := [][]string{
		[]string{
			"https://api.twitch.tv/helix/users/follows?first=1&to_id=" + ChannelID,
			"/webhooks/twitch/users/follows",
		},
		[]string{
			"https://api.twitch.tv/helix/subscriptions/events?first=1&broadcaster_id=" + ChannelID,
			"/webhooks/twitch/subscriptions/events",
		},
	}

	for _, pair := range topics {
		topic := pair[0]
		endpoint := pair[1]
		//TODO: eventually use Secret param too
		resp, err := currentTwitchClient.PostWebhookSubscription(&helix.WebhookSubscriptionPayload{
			Mode:         "subscribe",
			Topic:        topic,
			Callback:     config.ExternalURL + endpoint,
			LeaseSeconds: 24 * 60 * 60, // 24h is the max allowed
		})
		spew.Dump(resp)

		if err != nil {
			terrors.Log(err, "failed to create webhook subscription for "+endpoint)
		}
	}

	// talk to twitch and see what the current webhooks are
	//TODO: this doesnt seem to work, like at all
	resp, err := currentTwitchClient.GetWebhookSubscriptions(&helix.WebhookSubscriptionsParams{})
	if err != nil {
		terrors.Log(err, "failed to get webhook subscriptions")
	} else {
		if resp.Data.Total > 0 {
			spew.Dump(resp.Data.WebhookSubscriptions)
		} else {
			log.Println("no webhooks found")
		}
	}
	return
}
