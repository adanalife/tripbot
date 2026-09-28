package chatbot

import (
	"context"

	mylog "github.com/adanalife/tripbot/pkg/chatbot/log"
	"github.com/adanalife/tripbot/pkg/eventbus"
)

// ChatClient is the provider-neutral outbound chat surface the App sends
// through. Every platform reaches chat through the platform-gateway, which owns
// the credential and the transport, so a client here is a thin adapter over that
// call — gatewayChat for the platforms that can post, noOutboundChat for the
// ones that can't. Tests inject a recordingChat / noopChat fake.
type ChatClient interface {
	Say(msg string) // post a message in chat
	// Reply posts msg threaded under the message parentID, where the platform
	// can thread; elsewhere it posts msg plain. Callers reach it through
	// App.Reply, which knows the message being answered.
	Reply(parentID, msg string)
}

// consoleMirror wraps a ChatClient so the bot's own outbound messages are
// logged to Loki and mirrored onto the event bus — the admin live console,
// since streaming platforms don't echo our sent messages back to us. It's
// provider-neutral: every platform's Say() flows through here before reaching
// the platform client, so the console shows all of them uniformly.
type consoleMirror struct {
	inner       ChatClient
	env         string
	platform    string
	channel     string
	botUsername string
}

func (m consoleMirror) Say(msg string) {
	m.mirror(msg, nil)
	m.inner.Say(msg)
}

// Reply mirrors like Say, with the parent id on the bus line so the console
// can show which viewer's message the bot answered.
func (m consoleMirror) Reply(parentID, msg string) {
	m.mirror(msg, &eventbus.ChatReply{ParentMessageID: parentID})
	m.inner.Reply(parentID, msg)
}

func (m consoleMirror) mirror(msg string, reply *eventbus.ChatReply) {
	// include the message in the log
	mylog.ChatMsg(m.botUsername, m.channel, msg)
	// mirror the bot's own output onto the event bus so it shows in the admin
	// live console — the platform doesn't echo our sent messages back, so
	// without this the console would miss everything the bot says.
	// Fire-and-forget; no-op when NATS is unconfigured.
	eventbus.EmitChatMessage(context.Background(), m.env, eventbus.ChatMessage{
		Platform: m.platform,
		Username: m.botUsername,
		Text:     msg,
		Reply:    reply,
	})
}

// disconnectedChat is App.Chat's default between New() and the connect path,
// before a platform client is wired. Sends are dropped — in production nothing
// dispatches a command before the platform client is up, so it only ever covers
// that startup window (and any New()-built App a test doesn't override).
type disconnectedChat struct{}

func (disconnectedChat) Say(_ string)      {}
func (disconnectedChat) Reply(_, _ string) {}
