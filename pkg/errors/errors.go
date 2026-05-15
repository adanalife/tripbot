package errors

import (
	"errors"
	"fmt"
	"log"

	"github.com/adanalife/tripbot/pkg/config"
	"github.com/getsentry/sentry-go"
	"github.com/logrusorgru/aurora/v3"
)

var conf config.Config

// Initialize takes a Config interface and sets up a logger
func Initialize(c config.Config) {
	// sentry options are picked up through ENV vars
	err := sentry.Init(sentry.ClientOptions{
		// enable tracing
		TracesSampleRate: 0.2,
	})
	if err != nil {
		fmt.Println(err)
	}

	conf = c
}

//TODO: go through calls to this, find places we create a new Error, and change to nil
func Log(e error, msg string) {
	if e == nil {
		e = errors.New(msg)
	}
	// only log to sentry on production or staging
	if conf.IsProduction() || conf.IsStaging() {
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
	// only log to sentry on production or staging
	if conf.IsProduction() || conf.IsStaging() {
		sentry.AddBreadcrumb(&sentry.Breadcrumb{
			Message: msg,
		})
		sentry.CaptureException(e)
	}
	log.Fatalf("%s: %s", aurora.Red(msg), e)
}
