package chatEvents

import "fmt"

// domain is the fixed segment between <env> and the verb in the chat command
// subject: tripbot.<env>.chat.send. It matches the observation domain in
// pkg/eventbus (chat.message), so all chat subjects share the "chat" namespace
// and differ only by verb (send vs message).
const domain = "chat"

// SendSubject builds tripbot.<env>.chat.send — the operator "post this message"
// command. Subscribers (cmd/tripbot) build the same string to subscribe. The
// observation counterpart, chat.message, is eventbus.ChatMessageSubject.
func SendSubject(env string) string {
	return fmt.Sprintf("tripbot.%s.%s.send", env, domain)
}
