package instrumentation

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

var meter = otel.Meter("github.com/adanalife/tripbot")

var (
	chatMessages      = mustCounter("tripbot_chat_messages", "The total number of chat messages")
	chatCommands      = mustCounter("tripbot_chat_commands", "The total number of chat commands")
	twitchSubscribers = mustGauge("twitch_subscribers_total", "Current number of Twitch channel subscribers")
	twitchFollowers   = mustGauge("twitch_followers_total", "Current number of Twitch channel followers")
	obsStreamingGauge = mustGauge("obs_streaming_active", "1 if OBS is actively streaming, 0 otherwise")
)

// ChatMessages exposes the chat-message counter through a tiny stable API
// so call sites stay small (Inc()) and don't have to thread context.
var ChatMessages = chatCounterIface{counter: chatMessages}

// ChatCommands exposes the chat-command counter; record by calling
// ChatCommands.Inc(commandName).
var ChatCommands = chatCommandCounterIface{counter: chatCommands}

// TwitchAudience exposes subscriber and follower gauge recording.
var TwitchAudience = twitchAudienceIface{subscribers: twitchSubscribers, followers: twitchFollowers}

// OBSStreaming exposes the streaming-active gauge.
var OBSStreaming = obsStreamingIface{g: obsStreamingGauge}

type chatCounterIface struct{ counter metric.Int64Counter }

func (c chatCounterIface) Inc() {
	c.counter.Add(context.Background(), 1)
}

type chatCommandCounterIface struct{ counter metric.Int64Counter }

func (c chatCommandCounterIface) Inc(command string) {
	c.counter.Add(context.Background(), 1, metric.WithAttributes(attribute.String("command", command)))
}

type twitchAudienceIface struct {
	subscribers metric.Int64Gauge
	followers   metric.Int64Gauge
}

func (a twitchAudienceIface) SetSubscribers(n int64) {
	a.subscribers.Record(context.Background(), n)
}

func (a twitchAudienceIface) SetFollowers(n int64) {
	a.followers.Record(context.Background(), n)
}

type obsStreamingIface struct{ g metric.Int64Gauge }

func (o obsStreamingIface) Set(active bool) {
	v := int64(0)
	if active {
		v = 1
	}
	o.g.Record(context.Background(), v)
}

func mustCounter(name, desc string) metric.Int64Counter {
	c, err := meter.Int64Counter(name, metric.WithDescription(desc))
	if err != nil {
		panic(err)
	}
	return c
}

func mustGauge(name, desc string) metric.Int64Gauge {
	g, err := meter.Int64Gauge(name, metric.WithDescription(desc))
	if err != nil {
		panic(err)
	}
	return g
}
