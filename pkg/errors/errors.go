package errors

import (
	"errors"
	"fmt"
	"log"

	"github.com/adanalife/tripbot/pkg/config"
	"github.com/getsentry/sentry-go"
	sentryotel "github.com/getsentry/sentry-go/otel"
	"github.com/logrusorgru/aurora/v3"
)

var conf config.Config

// Initialize takes a Config interface and sets up a logger.
//
// version is the build-time version string (typically set via -ldflags
// "-X main.version=..." in cmd/tripbot and cmd/vlc-server). It's passed
// to sentry as the Release tag so Sentry can group issues by release
// and surface "this regression started in vX.Y.Z."
func Initialize(c config.Config, version string) {
	// Most sentry options (DSN, environment) are picked up through ENV
	// vars; Release is wired in explicitly so it tracks the same
	// build-time value the /version endpoint exposes.
	err := sentry.Init(sentry.ClientOptions{
		Release: version,
		// OTel is the source of truth for tracing (otelhttp + otelsql + manual
		// spans → OTLP → Tempo). Sentry's own tracer is left at the default
		// off-state to avoid double-tracking; the linking integration below
		// stamps the active OTel trace_id onto captured Sentry events so
		// errors clickthrough to their Tempo trace.
		Integrations: func(integrations []sentry.Integration) []sentry.Integration {
			return append(integrations, sentryotel.NewOtelIntegration())
		},
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
	// only log to sentry on production or staging; conf is nil in tests and
	// any binary that didn't call Initialize, so guard before the method call.
	if conf != nil && (conf.IsProduction() || conf.IsStaging()) {
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
	// only log to sentry on production or staging; conf is nil in tests and
	// any binary that didn't call Initialize, so guard before the method call.
	if conf != nil && (conf.IsProduction() || conf.IsStaging()) {
		sentry.AddBreadcrumb(&sentry.Breadcrumb{
			Message: msg,
		})
		sentry.CaptureException(e)
	}
	log.Fatalf("%s: %s", aurora.Red(msg), e)
}
