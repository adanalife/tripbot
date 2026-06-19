// Package chatEvents holds the wire format for the chat *command* events
// published over NATS — operator-initiated "send this message to chat"
// imperatives on subject tripbot.<env>.chat.send. This is the command
// counterpart to the observation chat.message events in pkg/eventbus: a Send
// says "post this", a ChatMessage reports "this was posted".
//
// It is imported by the subscriber (cmd/tripbot, which owns the Twitch
// identities and does the actual sending) and by whatever publishes the command
// — the standalone tripbot-console, once its chat-send feature lands (the
// in-tripbot panel that used to publish this was retired with the console
// split). Like pkg/onscreens-events and pkg/vlc-events it is
// stdlib-only and side-effect-free: no init(), no pkg/config import, env is
// always a parameter rather than read from config here — so it links safely
// into any binary.
package chatEvents

import "time"

// Identity selects which Twitch identity a Send is posted as. The values match
// the /auth/init account selector and pkg/twitch's AccountTokenStatus.Account,
// so the console's auth card and this command speak the same vocabulary.
const (
	IdentityBot         = "bot"
	IdentityBroadcaster = "broadcaster"
)

// Envelope is embedded in every chat command event. EmittedAt is an
// RFC3339Nano UTC timestamp, useful for latency/debugging. Snake_case JSON so a
// future protobuf schema maps 1-1.
type Envelope struct {
	EmittedAt string `json:"emitted_at"`
}

// NewEnvelope returns an Envelope stamped with the current UTC time.
func NewEnvelope() Envelope {
	return Envelope{EmittedAt: time.Now().UTC().Format(time.RFC3339Nano)}
}

// Send is the payload for the chat.send subject: post Text to the channel as
// the given Identity (IdentityBot | IdentityBroadcaster). Fire-and-forget —
// the sent line surfaces back in the live console through the normal
// chat.message path (the bot's send mirror, or the bot reading a
// broadcaster-sent line back inbound), so no reply is published here.
type Send struct {
	Envelope
	Identity string `json:"identity"`
	Text     string `json:"text"`
}
