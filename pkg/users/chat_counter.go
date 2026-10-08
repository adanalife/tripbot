package users

// ChatCounter is the read side of the per-tick chat-message tally the chatbot
// increments once per inbound message. The session tick drains it into each
// viewer_samples row, so the count is messages-per-sampling-window. It is
// optional: a Sessions with none wired records NULL chat_messages, which is
// distinct from a wired-and-silent 0.
type ChatCounter interface {
	// Drain returns the messages accumulated since the last call and resets
	// the tally.
	Drain() int
}
