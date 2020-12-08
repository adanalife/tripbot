package log

import (
	"context"
	"log"

	"cloud.google.com/go/logging"
	c "github.com/adanalife/tripbot/pkg/config/tripbot"
)

var client *logging.Client
var chatLogger *log.Logger

func init() {
	var err error

	// don't bother with this if we're in a test environment
	if c.IsTesting() || c.IsDevelopment() {
		return
	}

	ctx := context.Background()

	// Sets your Google Cloud Platform project ID.
	projectID := c.Conf.GoogleProjectID

	// Creates a stackdriver logging client.
	client, err = logging.NewClient(ctx, projectID)
	if err != nil {
		log.Fatalf("Failed to create stackdriver client: %v", err)
	}
	// defer client.Close()

	// this will include all Twitch chat messages
	chatLogger = client.Logger("twitch-chat").StandardLogger(logging.Info)

}

func ChatMsg(username, msg string) {
	if c.IsTesting() || c.IsDevelopment() {
		return
	}
	chatLogger.Printf("%s: %s", username, msg)
}
