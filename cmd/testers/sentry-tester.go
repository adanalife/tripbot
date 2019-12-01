package main

import (
	"errors"
	"os"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load()
	sentry.Init(sentry.ClientOptions{
		Dsn: os.Getenv("SENTRY_DSN"),
	})
	sentry.CaptureException(errors.New("my error2"))
	// Since sentry emits events in the background we need to make sure
	// they are sent before we shut down
	sentry.Flush(time.Second * 5)
}
