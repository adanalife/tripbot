package errors

import (
	"log"

	"github.com/getsentry/sentry-go"
	"github.com/logrusorgru/aurora"
	"github.com/spf13/cast"
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

func Log(e error, keyvals ...interface{}) {
	log.Printf("%s: %s", aurora.Red(msg), e)

	hub = sentry.CurrentHub()
	hub.WithScope(func(scope *sentry.Scope) {
		// Add given keyvals
		extra := make(map[string]interface{})
		for i := 0; i < len(keyvals)-1; i += 2 {
			key := cast.ToString(keyvals[i])
			if key == "" {
				continue
			}
			extra[key] = cast.ToString(keyvals[i+1])
		}
		scope.SetExtras(extra)
		hub.CaptureException(e)
	})
}

func Fatal(e error, msg string) {
	sentry.CaptureException(e)
	log.Fatalf("%s: %s", aurora.Red(msg), e)
}
