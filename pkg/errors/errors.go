package errors

import (
	"errors"
	"log"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/getsentry/sentry-go"
	"github.com/logrusorgru/aurora"
)

func init() {
	// sentry options are picked up through ENV vars
	sentry.Init(sentry.ClientOptions{})
}

//TODO: go through calls to this, find places we create a new Error, and change to nil
func Log(e error, msg string) {
	if e == nil {
		e = errors.New(msg)
	}
	// only log to sentry if on production
	if c.IsProduction() {
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
	if c.IsProduction() {
		sentry.AddBreadcrumb(&sentry.Breadcrumb{
			Message: msg,
		})
		sentry.CaptureException(e)
	}
	log.Fatalf("%s: %s", aurora.Red(msg), e)
}
