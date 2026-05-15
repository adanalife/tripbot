package chatbot

import "github.com/adanalife/tripbot/pkg/users"

// HandlerFunc is the common signature for all chat command handlers.
type HandlerFunc func(user *users.User, params []string)

// Command is a first-class representation of a single chat command.
type Command struct {
	Trigger            string
	Aliases            []string
	Handler            HandlerFunc
	RequiresFollow     bool
	RequiresSubscriber bool
}
