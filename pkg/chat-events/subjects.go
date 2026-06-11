package chatEvents

import "fmt"

// domain is the fixed segment between <env> and the verb in the chat command
// subject: tripbot.<env>.chat.send. It matches the observation domain in
// pkg/eventbus (chat.message), so all chat subjects share the "chat" namespace
// and differ only by verb (send vs message).
const domain = "chat"

// Platform values for the SendSubject platform segment. Per-platform
// tripbot instances subscribe only to their own platform's sends, so an
// admin send can never be double-handled (or handled by the wrong
// instance) in envs running more than one platform.
const (
	PlatformTwitch  = "twitch"
	PlatformYouTube = "youtube"
)

// SendSubject builds tripbot.<env>.chat.send.<platform> — the operator "post
// this message to <platform>'s chat" command. Subscribers (cmd/tripbot) build
// the same string to subscribe. The observation counterpart, chat.message, is
// eventbus.ChatMessageSubject (which carries platform in the payload instead:
// every console reads all platforms' lines, but a send targets exactly one).
func SendSubject(env, platform string) string {
	return fmt.Sprintf("tripbot.%s.%s.send.%s", env, domain, platform)
}
