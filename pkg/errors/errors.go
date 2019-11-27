package errors

import (
	"log"

	"github.com/getsentry/sentry-go"
	"github.com/logrusorgru/aurora"
)

func init() {
	// sentry options are picked up through ENV vars
	sentry.Init(sentry.ClientOptions{})
}

//TODO: include msg as an attribute somehow
func Log(e error, msg string) {
	sentry.AddBreadcrumb(&sentry.Breadcrumb{
		Message: msg,
	})
	sentry.CaptureException(e)
	log.Printf("%s: %s", aurora.Red(msg), e)
}

func Fatal(e error, msg string) {
	sentry.CaptureException(e)
	log.Fatalf("%s: %s", aurora.Red(msg), e)
}
