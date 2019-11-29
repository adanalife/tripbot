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
	//TODO: eventually use Secret param too
	_, err := currentTwitchClient.PostWebhookSubscription(&helix.WebhookSubscriptionPayload{
		Mode:         "subscribe",
		Topic:        "https://api.twitch.tv/helix/users/follows?first=1&to_id=" + ChannelID,
		Callback:     config.ExternalURL + "/webhooks/twitch/users/follows",
		LeaseSeconds: 24 * 60 * 60, // 24h is the max allowed
	})
	if err != nil {
		terrors.Log(err, "failed to create webhook subscription")
	}

	// talk to twitch and see what the current webhooks are
	//TODO: this doesnt seem to work, like at all
	resp, err := currentTwitchClient.GetWebhookSubscriptions(&helix.WebhookSubscriptionsParams{
		First: 10,
	})
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
