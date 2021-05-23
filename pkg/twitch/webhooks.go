package twitch

import (
	"log"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	terrors "github.com/adanalife/tripbot/pkg/errors"
	"github.com/davecgh/go-spew/spew"
	"github.com/logrusorgru/aurora"
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
	if c.Conf.DisableTwitchWebhooks {
		return
	}
	subscribeToWebhook(followsTopic)
	// since the staging account isn't an affiliate, don't bother
	if c.Conf.IsProduction() {
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
		Callback:     c.Conf.ExternalURL + endpoint,
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
		if c.Conf.Verbose {
			spew.Dump(resp.Data.WebhookSubscriptions)
		}
	} else {
		log.Println(aurora.Red("no webhooks found"))
	}
}
