// Package telemetry initializes OpenTelemetry providers (traces, metrics,
// logs) for tripbot's binaries. Backend is OTLP/HTTP — typically Grafana
// Cloud. Configuration comes from standard OTEL_* env vars.
//
// Setting OTEL_SDK_DISABLED=true (or leaving OTEL_EXPORTER_OTLP_ENDPOINT
// unset) makes Init skip the OTLP exporters and only wire up a Prometheus
// reader, so /metrics still works for local dev without spamming an
// endpoint that isn't there.
package telemetry

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/getsentry/sentry-go"
	sentryslog "github.com/samber/slog-sentry/v2"
	"go.opentelemetry.io/contrib/bridges/otelslog"
	otelruntime "go.opentelemetry.io/contrib/instrumentation/runtime"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/prometheus"
	logglobal "go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// ShutdownFunc flushes and tears down the providers. Wrap the passed
// context with a timeout so a wedged exporter can't block forever.
type ShutdownFunc func(context.Context) error

var noopShutdown ShutdownFunc = func(context.Context) error { return nil }

// Init wires up TracerProvider, MeterProvider, and LoggerProvider against
// the OTLP/HTTP endpoint described by OTEL_* env vars, plus a Prometheus
// exporter so the existing /metrics endpoint keeps serving. Always
// returns a non-nil ShutdownFunc — safe to defer unconditionally.
func Init(ctx context.Context, serviceName, serviceVersion string) (ShutdownFunc, error) {
	if disabled() {
		slog.InfoContext(ctx, "telemetry: OTLP exporters disabled, only Prometheus /metrics will be populated")
		if err := initPromOnlyMeter(ctx, serviceName, serviceVersion); err != nil {
			return noopShutdown, fmt.Errorf("prom-only meter: %w", err)
		}
		return noopShutdown, nil
	}

	res, err := newResource(ctx, serviceName, serviceVersion)
	if err != nil {
		return noopShutdown, fmt.Errorf("build resource: %w", err)
	}

	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	traceExp, err := otlptracehttp.New(ctx)
	if err != nil {
		return noopShutdown, fmt.Errorf("trace exporter: %w", err)
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExp),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)

	metricExp, err := otlpmetrichttp.New(ctx)
	if err != nil {
		return noopShutdown, fmt.Errorf("metric exporter: %w", err)
	}
	promExp, err := prometheus.New()
	if err != nil {
		return noopShutdown, fmt.Errorf("prometheus exporter: %w", err)
	}
	mpOpts := []sdkmetric.Option{
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExp)),
		sdkmetric.WithReader(promExp),
		sdkmetric.WithResource(res),
	}
	mpOpts = append(mpOpts, dropBodySizeHistograms()...)
	mp := sdkmetric.NewMeterProvider(mpOpts...)
	otel.SetMeterProvider(mp)

	if err := otelruntime.Start(otelruntime.WithMinimumReadMemStatsInterval(15 * time.Second)); err != nil {
		return noopShutdown, fmt.Errorf("runtime instrumentation: %w", err)
	}

	logExp, err := otlploghttp.New(ctx)
	if err != nil {
		return noopShutdown, fmt.Errorf("log exporter: %w", err)
	}
	lp := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(sdklog.NewBatchProcessor(logExp)),
		sdklog.WithResource(res),
	)
	logglobal.SetLoggerProvider(lp)

	consoleHandler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})
	otelHandler := otelslog.NewHandler(serviceName, otelslog.WithLoggerProvider(lp))
	// Sentry handler chain. Error+ becomes a captured event (subject to
	// the BeforeSend throttle in pkg/errors); Info/Warn becomes a
	// breadcrumb that attaches to the next event for context, no quota
	// cost. pkg/errors must call sentry.Init before any slog.Error fires
	// or events drop silently — cmd/* honors that ordering by calling
	// errors.Initialize right after telemetry.Init returns.
	sentryEventHandler := sentryslog.Option{
		Level:     slog.LevelError,
		AddSource: true,
	}.NewSentryHandler()
	sentryBreadcrumbHandler := breadcrumbHandler{}
	slog.SetDefault(slog.New(multiHandler{handlers: []slog.Handler{
		consoleHandler, otelHandler, sentryEventHandler, sentryBreadcrumbHandler,
	}}))

	// Redirect stdlib log.* through slog so existing call sites flow
	// through both the console and the OTel logger without rewriting them.
	log.SetFlags(0)
	log.SetOutput(slogWriter{level: slog.LevelInfo})

	return func(shutdownCtx context.Context) error {
		var errs []error
		if err := tp.Shutdown(shutdownCtx); err != nil {
			errs = append(errs, fmt.Errorf("tracer: %w", err))
		}
		if err := mp.Shutdown(shutdownCtx); err != nil {
			errs = append(errs, fmt.Errorf("meter: %w", err))
		}
		if err := lp.Shutdown(shutdownCtx); err != nil {
			errs = append(errs, fmt.Errorf("logger: %w", err))
		}
		return errors.Join(errs...)
	}, nil
}

// dropBodySizeHistograms returns MeterProvider options that drop the
// http_server_{request,response}_body_size_bytes histograms via OTel SDK
// views. These auto-instrumented histograms are pushed OTLP-direct to the
// metrics backend (bypassing Alloy, so they can't be relabeled chart-side)
// and account for ~700 active series with no panels reading them. The
// http_server_request_duration_seconds histogram is deliberately left
// untouched — its buckets back the p50/p95/p99 latency panels.
func dropBodySizeHistograms() []sdkmetric.Option {
	drop := func(name string) sdkmetric.Option {
		return sdkmetric.WithView(sdkmetric.NewView(
			sdkmetric.Instrument{Name: name},
			sdkmetric.Stream{Aggregation: sdkmetric.AggregationDrop{}},
		))
	}
	return []sdkmetric.Option{
		drop("http.server.request.body.size"),
		drop("http.server.response.body.size"),
	}
}

func disabled() bool {
	switch strings.ToLower(os.Getenv("OTEL_SDK_DISABLED")) {
	case "true", "1", "yes":
		return true
	}
	return os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") == ""
}

func newResource(ctx context.Context, name, version string) (*resource.Resource, error) {
	return resource.New(ctx,
		resource.WithFromEnv(),
		// Granular process detectors instead of resource.WithProcess(): the
		// bundled WithProcessOwner detector calls os/user.Current(), which
		// fails with "user: Current requires cgo or $USER set in environment"
		// in our static CGO_ENABLED=0 binaries running as a uid with no
		// /etc/passwd entry — that error silently disables the whole SDK.
		// Enumerate every process detector WithProcess() bundles *except*
		// the owner one, so process.* attributes are still emitted without
		// requiring a $USER workaround.
		resource.WithProcessPID(),
		resource.WithProcessExecutableName(),
		resource.WithProcessExecutablePath(),
		resource.WithProcessCommandArgs(),
		resource.WithProcessRuntimeName(),
		resource.WithProcessRuntimeVersion(),
		resource.WithProcessRuntimeDescription(),
		resource.WithHost(),
		resource.WithTelemetrySDK(),
		resource.WithAttributes(
			semconv.ServiceName(name),
			semconv.ServiceVersion(version),
		),
	)
}

func initPromOnlyMeter(ctx context.Context, name, version string) error {
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(name),
			semconv.ServiceVersion(version),
		),
	)
	if err != nil {
		return err
	}
	promExp, err := prometheus.New()
	if err != nil {
		return err
	}
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(promExp),
		sdkmetric.WithResource(res),
	)
	otel.SetMeterProvider(mp)
	if err := otelruntime.Start(otelruntime.WithMinimumReadMemStatsInterval(15 * time.Second)); err != nil {
		return fmt.Errorf("runtime instrumentation: %w", err)
	}
	return nil
}

// slogWriter adapts io.Writer to slog at a fixed level so stdlib log
// output flows through the configured slog handler chain.
type slogWriter struct{ level slog.Level }

func (w slogWriter) Write(p []byte) (int, error) {
	msg := strings.TrimRight(string(p), "\n")
	slog.Default().Log(context.Background(), w.level, msg)
	return len(p), nil
}

// multiHandler fans slog records out to multiple handlers.
type multiHandler struct{ handlers []slog.Handler }

func (m multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, h := range m.handlers {
		if h.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (m multiHandler) Handle(ctx context.Context, r slog.Record) error {
	var errs []error
	for _, h := range m.handlers {
		if h.Enabled(ctx, r.Level) {
			if err := h.Handle(ctx, r.Clone()); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

func (m multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	hs := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		hs[i] = h.WithAttrs(attrs)
	}
	return multiHandler{handlers: hs}
}

func (m multiHandler) WithGroup(name string) slog.Handler {
	hs := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		hs[i] = h.WithGroup(name)
	}
	return multiHandler{handlers: hs}
}

// breadcrumbHandler captures Info / Warn slog records as Sentry
// breadcrumbs (zero quota cost) so that when an Error fires later,
// the Sentry event arrives with the preceding log trail attached.
// Error+ is skipped — samber/slog-sentry already turns those into
// full Sentry events with their own captured attribute set.
//
// Sentry caps breadcrumbs at 100 per scope; high-volume routine logs
// (per-command "ran !X", per-cron-tick "session snapshot", etc.) would
// quickly evict the recent, useful records before an error fires. The
// noisy-prefix list below names the messages we know fire on every
// command or every minute via cron; they stay in Loki but skip the
// breadcrumb pipeline.
type breadcrumbHandler struct{}

// breadcrumbSkipPrefixes lists slog message prefixes that fire frequently
// enough to drown out other context if all routed to Sentry breadcrumbs.
var breadcrumbSkipPrefixes = []string{
	"ran !",            // every chat command (chatbot/commands.go + playback.go)
	"now playing",      // per video transition via cron.video.GetCurrentlyPlaying
	"session snapshot", // every 5min via cron.users.PrintCurrentSession
	"subscribers",      // every 5min via cron.twitch.GetSubscribers
	"no subscribers",   // ditto
	"follower count",   // every 5min via cron.twitch.GetFollowerCount
}

func (breadcrumbHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= slog.LevelInfo && level < slog.LevelError
}

func (breadcrumbHandler) Handle(ctx context.Context, r slog.Record) error {
	for _, p := range breadcrumbSkipPrefixes {
		if strings.HasPrefix(r.Message, p) {
			return nil
		}
	}
	hub := sentry.CurrentHub()
	if fromCtx := sentry.GetHubFromContext(ctx); fromCtx != nil {
		hub = fromCtx
	}
	hub.AddBreadcrumb(&sentry.Breadcrumb{
		Type:      "default",
		Category:  strings.ToLower(r.Level.String()),
		Message:   r.Message,
		Level:     sentryLevelFor(r.Level),
		Timestamp: r.Time,
	}, nil)
	return nil
}

func (h breadcrumbHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h breadcrumbHandler) WithGroup(_ string) slog.Handler      { return h }

func sentryLevelFor(l slog.Level) sentry.Level {
	switch {
	case l >= slog.LevelError:
		return sentry.LevelError
	case l >= slog.LevelWarn:
		return sentry.LevelWarning
	case l >= slog.LevelInfo:
		return sentry.LevelInfo
	default:
		return sentry.LevelDebug
	}
}
