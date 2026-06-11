package users

import "github.com/adanalife/tripbot/pkg/twitch"

// ChatterSource supplies the platform-specific view of who is currently in
// chat and each viewer's relationship to the channel. It is the seam between
// session tracking (platform-agnostic) and the chat transport (per-platform).
//
// Today the only implementation is twitchSource, backed by Twitch Helix via
// pkg/twitch. A YouTube or TikTok adapter drops in here so a per-platform bot
// instance (PLATFORM=youtube, etc.) tracks its own audience without Sessions
// changing.
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
	// IsFollower reports whether the user follows the channel.
	IsFollower(username string) bool
}

// twitchSource is the Twitch-backed ChatterSource. Its methods delegate to
// pkg/twitch's package surface; this adapter is the single point of
// users->twitch coupling, isolating it for the future per-platform split.
type twitchSource struct{}

func (twitchSource) UpdateChatters()               { twitch.UpdateChatters() }
func (twitchSource) Chatters() map[string]struct{} { return twitch.Chatters() }
func (twitchSource) ChatterCount() int             { return twitch.ChatterCount() }
func (twitchSource) IsSubscriber(username string) bool {
	return twitch.UserIsSubscriber(username)
}
func (twitchSource) IsFollower(username string) bool {
	return twitch.UserIsFollower(username)
}
