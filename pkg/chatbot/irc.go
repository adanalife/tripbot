package chatbot

// IRC is the subset of the IRC client surface chatbot commands depend on
// to send chat output. Tests inject a fake; production uses the package-backed
// realIRC adapter wired in New(). Mirrors the Onscreens/VLC pattern.
type IRC interface {
	Say(msg string)               // post a message in chat
	Whisper(username, msg string) // whisper to a specific user
}

// realIRC delegates to the package-level Say/Whisper so ALL bot chat output
// flows through one path: the Loki log line, the event-bus mirror (so the admin
// live console shows the bot's own output — Twitch doesn't echo it back), and
// the actual client.Say. This adapter previously duplicated Say()'s body and so
// skipped the event-bus emit, leaving command responses (which go through
// a.IRC.Say) out of the live console.
type realIRC struct{}

func (realIRC) Say(msg string)               { Say(msg) }
func (realIRC) Whisper(username, msg string) { Whisper(username, msg) }
