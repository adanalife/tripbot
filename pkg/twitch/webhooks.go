package twitch

import (
	"context"
	"log/slog"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/davecgh/go-spew/spew"
	"github.com/nicklaw5/helix/v2"
)

var followsTopic = []string{
	"https://api.twitch.tv/helix/users/follows?first=1&to_id=" + ChannelID,
	"/webhooks/twitch/users/follows",
}
var subsTopic = []string{
	"https://api.twitch.tv/helix/subscriptions/events?first=1&broadcaster_id=" + ChannelID,
	"/webhooks/twitch/subscriptions/events",
}

// UpdateWebhookSubscriptions will create new webhook subscriptions.
// ctx is forward-compat plumbing (see GetSubscribers).
func UpdateWebhookSubscriptions(ctx context.Context) {
	if c.Conf.DisableTwitchWebhooks {
		return
	}
	subscribeToWebhook(ctx, followsTopic)
	// since the staging account isn't an affiliate, don't bother
	if c.Conf.IsProduction() {
		subscribeToWebhook(ctx, subsTopic)
	}
	getWebookSubscriptions(ctx)
}

func subscribeToWebhook(ctx context.Context, pair []string) {
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
		slog.ErrorContext(ctx, "failed to create webhook subscription", "err", err, "endpoint", endpoint)
	}
}

func getWebookSubscriptions(ctx context.Context) {
	// talk to twitch and see what the current webhooks are
	//TODO: this doesn't seem to work, like at all
	resp, err := currentTwitchClient.GetWebhookSubscriptions(&helix.WebhookSubscriptionsParams{})
	if err != nil {
		slog.ErrorContext(ctx, "failed to get webhook subscriptions", "err", err)
	}

	if resp.Data.Total > 0 {
		if c.Conf.Verbose {
			spew.Dump(resp.Data.WebhookSubscriptions)
		}
	} else {
		slog.WarnContext(ctx, "no webhooks found")
	}
}
