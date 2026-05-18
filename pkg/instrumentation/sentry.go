package instrumentation

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// sentryEventsDropped counts events the pkg/errors BeforeSend throttle
// suppressed before they reached Sentry. Labeled by reason so Grafana
// can show cooldown-vs-cap separately.
var sentryEventsDropped = mustCounter(
	"sentry_events_dropped_total",
	"Sentry events dropped by the BeforeSend throttle, labeled by reason (cooldown|cap|disabled)",
)

// SentryEventsDropped exposes the drop counter; call SentryEventsDropped.Inc(reason)
// from inside BeforeSend when an event is being suppressed.
var SentryEventsDropped = sentryDroppedIface{counter: sentryEventsDropped}

type sentryDroppedIface struct{ counter metric.Int64Counter }

func (s sentryDroppedIface) Inc(reason string) {
	s.counter.Add(context.Background(), 1, metric.WithAttributes(attribute.String("reason", reason)))
}
