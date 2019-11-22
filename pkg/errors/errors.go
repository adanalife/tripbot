package errors

import (
	"log"
	"os"

	"github.com/getsentry/sentry-go"
	"github.com/logrusorgru/aurora"
)

func init() {
	// sentryDsn := os.Getenv("SENTRY_DSN")
	// if sentryDsn == "" {
	// 	log.Fatal("you must set SENTRY_DSN")
	// }
	// sentry.Init(sentry.ClientOptions{
	// 	Dsn: os.Getenv("SENTRY_DSN"),
	// })
	sentry.Init(sentry.ClientOptions{})
}

func Log(e error, msg string) {
	sentry.CaptureException(e)
	log.Printf("%s: %s", aurora.Red(msg), e)
}

func Fatal(e error, msg string) {
	sentry.CaptureException(e)
	log.Fatalf("%s: %s", aurora.Red(msg), e)
}
