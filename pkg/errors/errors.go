// Package errors initializes Sentry and exposes thin Fatal helpers.
//
// Error capture flows through slog: log emitters call slog.Error /
// slog.ErrorContext; pkg/telemetry installs samber/slog-sentry as a
// handler so every slog Error becomes a Sentry event automatically.
// The BeforeSend hook below throttles Sentry traffic to stay within
// the free-tier 5k events/month budget.
package errors

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/adanalife/tripbot/pkg/config"
	"github.com/adanalife/tripbot/pkg/contract"
	"github.com/adanalife/tripbot/pkg/instrumentation"
	"github.com/getsentry/sentry-go"
	sentryotel "github.com/getsentry/sentry-go/otel"
)

// Throttle settings. The free-tier budget is 5k events/month; flapping
// errors (OBS poll, video-script cron) can easily blow that in days
// without a cap. fingerprintCooldown collapses repeats of the same
// message; hourlyCap is an absolute belt-and-suspenders limit.
const (
	fingerprintCooldown = 15 * time.Minute
	hourlyCap           = 20
)

// Initialize takes a Config interface and brings up Sentry.
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
		BeforeSend: throttle(c),
	})
	if err != nil {
		fmt.Println(err)
	}

	// Tag the scope with which streaming platform this instance serves so
	// twitch vs youtube errors are filterable within the one shared Sentry
	// project. tripbot/vlc-server carry it as STREAM_PLATFORM; onscreens-server
	// as PLATFORM. Skip the tag when neither is set rather than tag an empty.
	platform := os.Getenv(contract.EnvKeyStreamPlatform)
	if platform == "" {
		platform = os.Getenv("PLATFORM")
	}
	if platform != "" {
		sentry.ConfigureScope(func(s *sentry.Scope) {
			s.SetTag("platform", platform)
		})
	}
}

// throttle returns a BeforeSend hook. In dev / testing it drops every
// event so local runs never reach the prod Sentry project. In prod /
// staging it enforces per-fingerprint cooldown + absolute hourly cap.
//
// Events dropped here still reach Loki via the OTel slog handler — Loki
// has the complete record; Sentry receives a deduplicated sample.
func throttle(c config.Config) func(event *sentry.Event, hint *sentry.EventHint) *sentry.Event {
	if c == nil || (!c.IsProduction() && !c.IsStaging()) {
		return func(*sentry.Event, *sentry.EventHint) *sentry.Event {
			instrumentation.SentryEventsDropped.Inc("disabled")
			return nil
		}
	}
	var (
		mu          sync.Mutex
		lastSent    = make(map[string]time.Time)
		windowStart = time.Now()
		windowCount int
	)
	return func(event *sentry.Event, _ *sentry.EventHint) *sentry.Event {
		mu.Lock()
		defer mu.Unlock()
		now := time.Now()
		if now.Sub(windowStart) > time.Hour {
			windowStart = now
			windowCount = 0
		}
		if windowCount >= hourlyCap {
			instrumentation.SentryEventsDropped.Inc("cap")
			return nil
		}
		// event.Message is the slog record's Message field when sent via
		// samber/slog-sentry. Group by it so repeats of the same error
		// collapse to one event per cooldown window.
		fp := event.Message
		if t, ok := lastSent[fp]; ok && now.Sub(t) < fingerprintCooldown {
			instrumentation.SentryEventsDropped.Inc("cooldown")
			return nil
		}
		lastSent[fp] = now
		windowCount++
		return event
	}
}

// Fatal logs msg + err at Error level (which reaches Sentry via the
// slog handler) and exits with status 1. The ctx-less form is for
// startup / shutdown paths where no parent span exists.
func Fatal(e error, msg string) {
	FatalContext(context.Background(), e, msg)
}

// FatalContext is the trace-aware sibling of Fatal.
func FatalContext(ctx context.Context, e error, msg string) {
	if e == nil {
		e = errors.New(msg)
	}
	slog.ErrorContext(ctx, msg, "err", e)
	// The slog-sentry handler enqueues on an async transport; without a
	// flush the fatal event would die with the process before it's sent.
	sentry.Flush(2 * time.Second)
	os.Exit(1)
}
