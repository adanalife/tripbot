package users

// ChatterSource supplies the platform-specific view of who is currently in
// chat and each viewer's relationship to the channel. It is the seam between
// session tracking (platform-agnostic) and the chat transport (per-platform).
//
// The production implementation is cmd/tripbot's gatewayChatterSource, backed
// by the platform-gateway. A YouTube or TikTok adapter drops in here so a
// per-platform bot instance tracks its own audience without Sessions changing.
type ChatterSource interface {
	// UpdateChatters refreshes the source's notion of who is in chat.
	UpdateChatters()
	// Chatters returns the set of usernames currently in chat.
	Chatters() map[string]struct{}
	// ChatterCount is the authoritative in-chat total. It can exceed the
	// number of logged-in users (e.g. lurkers the source counts but that
	// never appear in chat).
	ChatterCount() int
	// IsSubscriber reports whether the user is a paid subscriber/member.
	IsSubscriber(username string) bool
	// SubscriberTier reports the user's paid subscription tier (1–3 on
	// Twitch), or 0 for a non-subscriber.
	SubscriberTier(username string) int
	// IsFollower reports whether the user follows the channel.
	IsFollower(username string) bool
}
