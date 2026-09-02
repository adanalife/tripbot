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
	RequiresAdmin      bool

	// Help is the one-line answer to "!help <trigger>": what the command does,
	// in chat-sized viewer-facing prose. Every command sets it — the registry
	// test fails on an empty one — so the man page can't lag the surface.
	Help string

	// AdminDeniedMsg is what chat hears when a non-admin runs this command.
	// Empty is the default and the common case: an admin command declines in
	// silence rather than advertising itself.
	AdminDeniedMsg string

	// Platforms restricts a command to specific streaming platforms. Leave it
	// nil for a cross-platform command (the common case). Set it for a command
	// that only makes sense on certain platforms — e.g. !carsound repoints an
	// OBS source that only the YouTube scene has, so it's
	// Platforms: []string{platformYouTube}. This is the tidy home for
	// platform-specific commands: the scope lives next to the handler, is
	// symmetric across platforms (no single platform is privileged), and a
	// future Kick/TikTok-only command just lists its own platform here. A
	// command with a non-nil Platforms is governed solely by it — see
	// (*App).commandEnabled.
	Platforms []string
}
