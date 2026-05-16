package chatbot

import (
	"context"

	"github.com/adanalife/tripbot/pkg/users"
)

// HandlerFunc is the common signature for all chat command handlers. The
// ctx carries the chat.command span started in dispatch() so SQL queries
// (via otelsql) and outbound HTTP (via otelhttp) nest under the command
// span in Tempo.
type HandlerFunc func(ctx context.Context, user *users.User, params []string)

// Command is a first-class representation of a single chat command.
type Command struct {
	Trigger            string
	Aliases            []string
	Handler            HandlerFunc
	RequiresFollow     bool
	RequiresSubscriber bool
}
