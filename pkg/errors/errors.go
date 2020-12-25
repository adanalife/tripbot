package errors

import (
	"errors"
	"log"

	"github.com/adanalife/tripbot/pkg/config"
	"github.com/getsentry/sentry-go"
	"github.com/logrusorgru/aurora"
)

var conf config.Config

func Init(c config.Config) {
	// sentry options are picked up through ENV vars
	sentry.Init(sentry.ClientOptions{})

	conf = c
}

//TODO: go through calls to this, find places we create a new Error, and change to nil
func Log(e error, msg string) {
	if e == nil {
		e = errors.New(msg)
	}
	// only log to sentry if on production
	if conf.IsProduction() {
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
	if conf.IsProduction() {
		sentry.AddBreadcrumb(&sentry.Breadcrumb{
			Message: msg,
		})
		sentry.CaptureException(e)
	}
	log.Fatalf("%s: %s", aurora.Red(msg), e)
}
