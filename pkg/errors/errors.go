package errors

import (
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/adanalife/tripbot/pkg/config"
	"github.com/getsentry/sentry-go"
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
	// only log to sentry on production or staging; conf is nil in tests and
	// any binary that didn't call Initialize, so guard before the method call.
	if conf != nil && (conf.IsProduction() || conf.IsStaging()) {
		sentry.AddBreadcrumb(&sentry.Breadcrumb{
			Message: msg,
		})
		sentry.CaptureException(e)
	}
	slog.Error(msg, "err", e)
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
	slog.Error(msg, "err", e)
	os.Exit(1)
}
