package log

import (
	"context"
	"log"

	"cloud.google.com/go/logging"
	"github.com/adanalife/tripbot/internal/config"
)

var client *logging.Client
var chatLogger *log.Logger

func init() {
	var err error

	// don't bother with this if we're in a test environment
	if config.IsTesting() || config.IsDevelopment() {
		return
	}

	ctx := context.Background()

	// Sets your Google Cloud Platform project ID.
	projectID := config.GoogleProjectID

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
	if config.IsTesting() || config.IsDevelopment() {
		return
	}
	chatLogger.Printf("%s: %s", username, msg)
}
