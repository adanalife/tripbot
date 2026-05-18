package chatbot

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	mylog "github.com/adanalife/tripbot/pkg/chatbot/log"
	c "github.com/adanalife/tripbot/pkg/config/tripbot"
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
// It calls sayFn with the appropriate denial message when access is denied.
func (cmd *Command) checkAccess(ctx context.Context, user chatUser, sayFn func(string)) bool {
	if cmd.RequiresFollow && !user.HasCommandAvailable(ctx) {
		sayFn(followerMsg)
		return false
	}
	if cmd.RequiresSubscriber && !user.IsSubscriber() {
		sayFn(subscriberMsg)
		return false
	}
	return true
}

func dispatch(ctx context.Context, cmd *Command, user *users.User, params []string) {
	incChatCommandCounter(cmd.Trigger)
	if !cmd.checkAccess(ctx, user, sayFn) {
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
func findCommand(message string) (*Command, []string) {
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
	for alias, cmd := range multiWordLookup {
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
	if cmd, ok := singleWordLookup[command]; ok {
		return cmd, params
	}
	return nil, nil
}

func runCommand(ctx context.Context, user *users.User, message string) {
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

	cmd, params := findCommand(message)
	if cmd != nil {
		dispatch(ctx, cmd, user, params)
		return
	}

	if strings.HasPrefix(command, "!") {
		err := fmt.Errorf("command %s not found", command)
		slog.ErrorContext(ctx, "error running command", "err", err)
	}
}

// handles all chat messages
func PrivateMessage(msg twitch.PrivateMessage) {
	username := msg.User.Name

	ctx, span := tracer.Start(context.Background(), "chatbot.handle_message",
		trace.WithAttributes(attribute.String("twitch.user", username)))
	defer span.End()

	// increment the Prometheus counter
	instrumentation.ChatMessages.Inc()

	//TODO: we lose capitalization here, is that okay?
	message := strings.ToLower(msg.Message)

	// emit chat line to Loki via OTel
	mylog.ChatMsg(username, msg.Message)

	// check to see if the message is a command
	//TODO: also include ones prefixed with whitespace?
	// log in the user
	user := users.LoginIfNecessary(ctx, username)

	runCommand(ctx, user, message)
}

// this event fires when a user joins the channel
func UserJoin(joinMessage twitch.UserJoinMessage) {
	users.LoginIfNecessary(context.Background(), joinMessage.User)
}

// this event fires when a user leaves the channel
func UserPart(partMessage twitch.UserPartMessage) {
	users.LogoutIfNecessary(context.Background(), partMessage.User)
}

// send message to chat if someone subs
// func UserNotice(message twitch.UserNoticeMessage) {
// 	// update the internal subscriber list
// 	mytwitch.GetSubscribers()
// }

// if the message comes from me, then post the message to chat.
// An admin whisper that triggers Say() is logged again as a chat line.
func GetWhisper(message twitch.WhisperMessage) {
	slog.Info("whisper received", "from", message.User.Name, "msg", message.Message)
	if c.UserIsAdmin(message.User.Name) {
		Say(message.Message)
	}
}
