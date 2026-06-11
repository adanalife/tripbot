package chatbot

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	mylog "github.com/adanalife/tripbot/pkg/chatbot/log"
	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/eventbus"
	"github.com/adanalife/tripbot/pkg/instrumentation"
	"github.com/adanalife/tripbot/pkg/users"
	"github.com/gempir/go-twitch-irc/v4"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

var tracer = otel.Tracer("github.com/adanalife/tripbot/pkg/chatbot")

func incChatCommandCounter(command string) {
	instrumentation.ChatCommands.Inc(command)
}

// normalizeCommandPrefix rewrites a leading Spanish inverted exclamation mark
// `¡` (U+00A1, two bytes in UTF-8: 0xC2 0xA1) to a regular `!` so that
// Spanish-keyboard users (who type `¡` where US keyboards type `!`) can run
// commands without switching layouts. e.g. `¡miles` -> `!miles`.
func normalizeCommandPrefix(msg string) string {
	if strings.HasPrefix(msg, "¡") {
		return "!" + strings.TrimPrefix(msg, "¡")
	}
	return msg
}

// chatUser is the subset of *users.User that dispatch needs for access checks.
type chatUser interface {
	HasCommandAvailable(ctx context.Context) bool
	IsSubscriber() bool
}

// checkAccess returns true when the user is allowed to run cmd.
// It calls say with the appropriate denial message when access is denied.
func (cmd *Command) checkAccess(ctx context.Context, user chatUser, say func(string)) bool {
	if followerGatingEnabled && cmd.RequiresFollow && !user.HasCommandAvailable(ctx) {
		say(followerMsg)
		return false
	}
	if cmd.RequiresSubscriber && !user.IsSubscriber() {
		say(subscriberMsg)
		return false
	}
	return true
}

// sessionUser adapts a *users.User plus the installed *Sessions to the
// chatUser access-check seam — the follower/subscriber + command-availability
// checks now live on Sessions (per-provider state), not on User.
type sessionUser struct {
	s *users.Sessions
	u *users.User
}

func (su sessionUser) HasCommandAvailable(ctx context.Context) bool {
	return su.s.HasCommandAvailable(ctx, su.u)
}
func (su sessionUser) IsSubscriber() bool { return su.s.IsSubscriber(*su.u) }

func (a *App) dispatch(ctx context.Context, cmd *Command, user *users.User, params []string) {
	incChatCommandCounter(cmd.Trigger)
	if !cmd.checkAccess(ctx, sessionUser{a.UserSessions, user}, a.Chat.Say) {
		return
	}
	// Start a child span under the chatbot.handle_message span from
	// PrivateMessage. SQL queries (via otelsql) and outbound HTTP (via
	// otelhttp) nest under chat.command in Tempo, so a single !miles
	// shows up as one trace with all 4 GetScore-chain SQL spans nested.
	ctx, span := tracer.Start(ctx, "chat.command",
		trace.WithAttributes(attribute.String("command", cmd.Trigger)))
	defer span.End()

	start := time.Now()
	cmd.Handler(ctx, user, params)
	instrumentation.ChatCommandDuration.Observe(cmd.Trigger, time.Since(start).Seconds())
}

// findCommand parses message and returns the matching Command and params.
// Returns nil if no command matches.
func (a *App) findCommand(message string) (*Command, []string) {
	msg := normalizeCommandPrefix(strings.TrimSpace(message))
	split := strings.Split(msg, " ")

	command := split[0]
	var params []string

	if len(split) > 1 {
		params = split[1:]

		// this invalid unicode character shows up when you run the same command twice
		// (it may be specific to Chatterino as a twitch client?)
		if params[len(params)-1] == "\U000e0000" {
			params = params[:len(params)-1]
		}
	}

	// handle case where people add a space (like "! location")
	if command == "!" && len(params) > 0 {
		command = command + params[0]
		params = params[1:]
	}

	// multi-word alias lookup (e.g. "no audio", "no sound")
	for alias, cmd := range a.multiWordLookup {
		if msg == alias || strings.HasPrefix(msg, alias+" ") {
			remainder := strings.TrimSpace(strings.TrimPrefix(msg, alias))
			var mwParams []string
			if remainder != "" {
				mwParams = strings.Split(remainder, " ")
			}
			return cmd, mwParams
		}
	}

	// single-word lookup
	if cmd, ok := a.singleWordLookup[command]; ok {
		return cmd, params
	}
	return nil, nil
}

func (a *App) runCommand(ctx context.Context, user *users.User, message string) {
	// parse for otel span attribute (only set for !-prefixed commands)
	msg := normalizeCommandPrefix(strings.TrimSpace(message))
	split := strings.Split(msg, " ")
	command := split[0]
	if command == "!" && len(split) > 1 {
		command = "!" + split[1]
	}
	// Tag the active span with the parsed command. Bare-word triggers
	// (e.g. "hello") aren't included to keep the attribute's cardinality
	// bounded to the bot's actual command surface (and typos thereof).
	if strings.HasPrefix(command, "!") {
		trace.SpanFromContext(ctx).SetAttributes(attribute.String("twitch.command", command))
	}

	cmd, params := a.findCommand(message)
	if cmd != nil {
		a.dispatch(ctx, cmd, user, params)
		return
	}

	if strings.HasPrefix(command, "!") {
		err := fmt.Errorf("command %s not found", command)
		slog.ErrorContext(ctx, "error running command", "err", err)
	}
}

// IncomingMessage is a platform-neutral inbound chat message. Platform
// adapters (Twitch today; YouTube and others later) translate their native
// event types into this shape before handing it to the App's Handle* methods,
// so the command path stays platform-agnostic.
type IncomingMessage struct {
	User string // sender's platform username (Twitch sends display-case)
	Text string // the message body, original case
}

// HandleMessage processes one inbound chat message: records it (Loki + the
// admin-console event bus), logs the user in, and runs any command it carries.
func (a *App) HandleMessage(ctx context.Context, msg IncomingMessage) {
	// span attribute kept as twitch.user for observability continuity; it
	// generalizes to a platform-tagged key once a second platform lands.
	ctx, span := tracer.Start(ctx, "chatbot.handle_message",
		trace.WithAttributes(attribute.String("twitch.user", msg.User)))
	defer span.End()

	// increment the Prometheus counter
	instrumentation.ChatMessages.Inc()

	// emit chat line to Loki via OTel
	mylog.ChatMsg(msg.User, msg.Text)

	// mirror the chat line onto the event bus so live consumers (the admin
	// panel's chat pane) see it. Original-case username + text, matching the
	// Loki line above; fire-and-forget, no-op when NATS is unconfigured.
	eventbus.EmitChatMessage(ctx, c.Conf.Environment, a.Platform, msg.User, msg.Text)

	// log in the user, then run any command (lowercased for matching)
	//TODO: we lose capitalization here, is that okay?
	//TODO: also handle commands prefixed with whitespace?
	user := a.UserSessions.LoginIfNecessary(ctx, msg.User)
	a.runCommand(ctx, user, strings.ToLower(msg.Text))
}

// HandleJoin records that a user joined the channel.
func (a *App) HandleJoin(username string) {
	a.UserSessions.LoginIfNecessary(context.Background(), username)
}

// HandlePart records that a user left the channel.
func (a *App) HandlePart(username string) {
	a.UserSessions.LogoutIfNecessary(context.Background(), username)
}

// HandleWhisper lets an admin remote-say into chat by whispering the bot.
// The resulting Say() is logged again as a chat line.
func (a *App) HandleWhisper(msg IncomingMessage) {
	slog.Info("whisper received", "from", msg.User, "text", msg.Text)
	if c.UserIsAdmin(msg.User) {
		a.Chat.Say(msg.Text)
	}
}

// --- Twitch inbound adapters ---
//
// These translate go-twitch-irc event types into neutral Handle* calls on the
// App, and are wired to the IRC client in ConnectIRC. A future YouTube/etc.
// transport adds its own adapters feeding the same Handle* methods, so the
// command path never learns about platforms.

func (a *App) onTwitchMessage(msg twitch.PrivateMessage) {
	a.HandleMessage(context.Background(), IncomingMessage{User: msg.User.Name, Text: msg.Message})
}

func (a *App) onTwitchJoin(joinMessage twitch.UserJoinMessage) {
	a.HandleJoin(joinMessage.User)
}

func (a *App) onTwitchPart(partMessage twitch.UserPartMessage) {
	a.HandlePart(partMessage.User)
}

func (a *App) onTwitchWhisper(message twitch.WhisperMessage) {
	a.HandleWhisper(IncomingMessage{User: message.User.Name, Text: message.Message})
}
