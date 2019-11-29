package twitch

import (
	"log"
	"os/exec"
	"path"

	"github.com/davecgh/go-spew/spew"
	"github.com/dmerrick/danalol-stream/pkg/config"
	terrors "github.com/dmerrick/danalol-stream/pkg/errors"
	"github.com/dmerrick/danalol-stream/pkg/helpers"
	"github.com/nicklaw5/helix"
)

// UpdateWebhookSubscriptions will call a shell script (lol) to
// create new webhook subscriptions
//TODO: use the twitch API instead of a shell script when possible
func UpdateWebhookSubscriptions() {
	// run the shell script to update webhook subscriptions
	scriptPath := path.Join(helpers.ProjectRoot(), "bin/subscribe-to-webhooks.sh")
	_, err := exec.Command(scriptPath, config.ExternalURL).Output()
	if err != nil {
		terrors.Log(err, "failed to run script")
		return
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
