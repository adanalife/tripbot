package users

import "github.com/adanalife/tripbot/pkg/viewstats"

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
	// UpdateAudience refreshes the source's notion of how many people are
	// watching. Separate from UpdateChatters because it's a different
	// question and a different upstream call — a platform can answer one and
	// not the other.
	UpdateAudience()
	// Audience is the concurrent-viewer reading cached by UpdateAudience.
	// Reported is false where the platform publishes no such number, which
	// callers must not flatten to a count of zero.
	Audience() viewstats.Audience
}

// VideoSource supplies what is on screen right now, so a login/logout event
// records the footage the viewer arrived on or left on. That pairing is what
// turns the session log into per-clip churn — which footage holds an audience
// and which sheds it — a question the raw viewer counts can't answer.
//
// The production implementation is cmd/tripbot's gatewayVideoSource, reading
// the process-wide player. It is optional: a Sessions with no source recorded
// writes NULL airing context, which is correct for an instance that isn't
// driving playback.
//
// Both methods must be cheap and I/O-free — they run inline on every login and
// logout, and a session tick must never block on playout.
type VideoSource interface {
	// CurrentVideoID is the airing clip's videos.id, 0 when nothing is
	// playing or the clip has no DB row.
	CurrentVideoID() int
	// CurrentProgressSec is how many seconds into the airing clip the
	// playhead sits.
	CurrentProgressSec() float64
}
