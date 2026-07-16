package chatbot

import (
	"context"
	"fmt"
	"log/slog"

	mylog "github.com/adanalife/tripbot/pkg/chatbot/log"
	"github.com/adanalife/tripbot/pkg/eventbus"
	"github.com/gempir/go-twitch-irc/v4"
)

// ChatClient is the provider-neutral outbound chat surface the App sends
// through. twitchChat implements it for Twitch; a YouTube / TikTok client
// implements the same two methods and drops into App.Chat without any command
// code changing. Tests inject a recordingChat / noopChat fake.
type ChatClient interface {
	Say(msg string)               // post a message in chat
	Whisper(username, msg string) // whisper to a specific user
}

// consoleMirror wraps a ChatClient so the bot's own outbound messages are
// logged to Loki and mirrored onto the event bus — the admin live console,
// since streaming platforms don't echo our sent messages back to us. It's
// provider-neutral: every platform's Say() flows through here before reaching
// the platform client, so the console shows all of them uniformly. ConnectIRC
// wires the live one around twitchChat.
type consoleMirror struct {
	inner       ChatClient
	env         string
	platform    string
	channel     string
	botUsername string
}

func (m consoleMirror) Say(msg string) {
	// include the message in the log
	mylog.ChatMsg(m.botUsername, m.channel, msg)
	// mirror the bot's own output onto the event bus so it shows in the admin
	// live console — the platform doesn't echo our sent messages back, so
	// without this the console would miss everything the bot says.
	// Fire-and-forget; no-op when NATS is unconfigured.
	eventbus.EmitChatMessage(context.Background(), m.env, m.platform, m.botUsername, msg)
	m.inner.Say(msg)
}

// Whisper isn't mirrored to the console (whispers aren't channel chat); just
// pass it through to the platform client.
func (m consoleMirror) Whisper(username, msg string) {
	m.inner.Whisper(username, msg)
}

// twitchChat sends to Twitch through its own *twitch.Client and channel /
// identity config. It reads no package-level globals — ConnectIRC builds it
// with the App's client and the values from config, which is what makes a
// second provider additive (and pre-shapes the eventual out-of-process Helix /
// auth service that will own token provisioning).
type twitchChat struct {
	client      *twitch.Client
	channelName string
	botUsername string
}

func (tc twitchChat) Say(msg string) {
	tc.client.Say(tc.channelName, msg)
}

// Whisper replicates the v2 whisper behavior: go-twitch-irc v4 removed the
// Whisper() send method, so we send the raw IRC /w command via PRIVMSG on the
// bot's own channel.
func (tc twitchChat) Whisper(username, msg string) {
	slog.Info("sending whisper", "to", username, "text", msg)
	tc.client.Say(tc.botUsername, fmt.Sprintf("/w %s %s", username, msg))
}

// disconnectedChat is App.Chat's default between New() and ConnectIRC, before
// a platform client is wired. Sends are dropped — in production nothing
// dispatches a command before the IRC connection is up, so it only ever covers
// that startup window (and any New()-built App a test doesn't override).
type disconnectedChat struct{}

func (disconnectedChat) Say(_ string)        {}
func (disconnectedChat) Whisper(_, _ string) {}
