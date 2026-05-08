// Package telemetry initializes OpenTelemetry providers (traces, metrics,
// logs) for tripbot's binaries. Backend is OTLP/HTTP — typically Grafana
// Cloud. Configuration comes from standard OTEL_* env vars; see
// vault/decisions for the surrounding design.
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

	"go.opentelemetry.io/contrib/bridges/otelslog"
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
		log.Println("[telemetry] OTLP exporters disabled; only Prometheus /metrics will be populated")
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
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExp)),
		sdkmetric.WithReader(promExp),
		sdkmetric.WithResource(res),
	)
	otel.SetMeterProvider(mp)

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
	slog.SetDefault(slog.New(multiHandler{handlers: []slog.Handler{consoleHandler, otelHandler}}))

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
		resource.WithProcess(),
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
