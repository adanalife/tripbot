package instrumentation

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

var meter = otel.Meter("github.com/adanalife/tripbot")

var (
	chatMessages = mustCounter("tripbot_chat_messages", "The total number of chat messages")
	chatCommands = mustCounter("tripbot_chat_commands", "The total number of chat commands")
)

// ChatMessages exposes the chat-message counter through a tiny stable API
// so call sites stay small (Inc()) and don't have to thread context.
var ChatMessages = chatCounterIface{counter: chatMessages}

// ChatCommands exposes the chat-command counter; record by calling
// ChatCommands.Inc(commandName).
var ChatCommands = chatCommandCounterIface{counter: chatCommands}

type chatCounterIface struct{ counter metric.Int64Counter }

func (c chatCounterIface) Inc() {
	c.counter.Add(context.Background(), 1)
}

type chatCommandCounterIface struct{ counter metric.Int64Counter }

func (c chatCommandCounterIface) Inc(command string) {
	c.counter.Add(context.Background(), 1, metric.WithAttributes(attribute.String("command", command)))
}

func mustCounter(name, desc string) metric.Int64Counter {
	c, err := meter.Int64Counter(name, metric.WithDescription(desc))
	if err != nil {
		panic(err)
	}
	return c
}
