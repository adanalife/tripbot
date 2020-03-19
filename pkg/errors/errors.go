package errors

import (
	"errors"
	"log"

	"github.com/dmerrick/tripbot/pkg/config"
	"github.com/getsentry/sentry-go"
	"github.com/logrusorgru/aurora"
)

func init() {
	// sentry options are picked up through ENV vars
	sentry.Init(sentry.ClientOptions{})
}

func Log(e error, msg string) {
	if e == nil {
		e = errors.New(msg)
	}
	// only log to sentry if on production
	if config.IsProduction() {
		sentry.AddBreadcrumb(&sentry.Breadcrumb{
			Message: msg,
		})
		sentry.CaptureException(e)
	}
	log.Printf("%s: %s", aurora.Red(msg), e)
}

func Fatal(e error, msg string) {
	if e == nil {
		e = errors.New(msg)
	}
	// only log to sentry if on production
	if config.IsProduction() {
		sentry.AddBreadcrumb(&sentry.Breadcrumb{
			Message: msg,
		})
		sentry.CaptureException(e)
	}
	log.Fatalf("%s: %s", aurora.Red(msg), e)
}
