package chatbot

// ChatCounter is the write side of the per-tick chat-message tally that lands
// in viewer_samples.chat_messages. The chatbot only ever increments it; the
// session tick drains it (pkg/users), and cmd/tripbot hands the same
// viewstats.MessageCounter to both sides.
type ChatCounter interface {
	// Add counts one inbound chat message.
	Add()
}
