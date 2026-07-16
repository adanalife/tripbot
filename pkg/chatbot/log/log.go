// Package log emits Twitch chat messages as OTel log records so they
// flow to Grafana Cloud Loki alongside the rest of tripbot's telemetry.
// Querying:
//
//	{service_name="tripbot", scope_name="twitch-chat"}
//
// twitch.user and twitch.channel land as OTel attributes; the message
// text is the log Body. When OTLP is disabled (OTEL_SDK_DISABLED=true
// or no endpoint), the global LoggerProvider is the SDK's noop, so
// Emit is a no-op — chat lines stay off the wire and off the console.
package log

import (
	"context"

	otellog "go.opentelemetry.io/otel/log"
	logglobal "go.opentelemetry.io/otel/log/global"
)

const scopeName = "twitch-chat"

func ChatMsg(username, channel, msg string) {
	logger := logglobal.GetLoggerProvider().Logger(scopeName)

	var rec otellog.Record
	rec.SetBody(otellog.StringValue(msg))
	rec.SetSeverity(otellog.SeverityInfo)
	rec.AddAttributes(
		otellog.String("twitch.user", username),
		otellog.String("twitch.channel", channel),
	)
	logger.Emit(context.Background(), rec)
}
