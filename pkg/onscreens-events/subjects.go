package onscreensEvents

import "fmt"

// domain is the fixed segment between <env> and <overlay> in every
// onscreens subject: tripbot.<env>.onscreens.<overlay>.<verb>.<platform>.
const domain = "onscreens"

// subject builds tripbot.<env>.onscreens.<overlay>.<verb>.<platform>. The
// trailing platform leaf keeps each streaming platform's overlays isolated:
// every per-platform instance (tripbot-twitch, tripbot-youtube) shares the
// env's NATS, and each onscreens-server (onscreens-twitch, onscreens-youtube)
// subscribes only to its own leaf, so a Twitch-triggered overlay (a
// leaderboard, a timewarp) never renders on the YouTube stream — same shape as
// tripbot.<env>.vlc.lastplayed.<platform>. Unexported so callers go through
// the typed constructors below, keeping the overlay/verb strings in one place
// and out of reach of typos at the call site.
func subject(env, platform, overlay, verb string) string {
	return fmt.Sprintf("tripbot.%s.%s.%s.%s.%s", env, domain, overlay, verb, platform)
}

func MiddleShowSubject(env, platform string) string { return subject(env, platform, "middle", "show") }
func MiddleHideSubject(env, platform string) string { return subject(env, platform, "middle", "hide") }

// MiddleStateSubject is the last-value state leaf onscreens-server publishes
// the middle-text overlay's content + visibility to, so a restarted server
// restores it (tripbot.<env>.onscreens.middle.state.<platform>). Unlike
// middle.show / middle.hide (commands from tripbot) this is *state* the server
// emits about itself — backed by a MaxMsgsPerSubject=1 stream, the same shape
// as the vlc lastplayed cache (tripbot.<env>.vlc.lastplayed.<platform>). The
// platform leaf keeps onscreens-twitch and onscreens-youtube from clobbering
// one another's restore cache.
func MiddleStateSubject(env, platform string) string {
	return subject(env, platform, "middle", "state")
}

// MiddleStateWildcard covers every platform's middle.state leaf in env — the
// subject filter the TRIPBOT_ONSCREENS_MIDDLE stream is declared with, so both
// per-platform servers can idempotently ensure the one stream while each reads
// and writes only its own leaf.
func MiddleStateWildcard(env string) string { return subject(env, "*", "middle", "state") }

func LeaderboardShowSubject(env, platform string) string {
	return subject(env, platform, "leaderboard", "show")
}
func LeaderboardHideSubject(env, platform string) string {
	return subject(env, platform, "leaderboard", "hide")
}
func TimewarpShowSubject(env, platform string) string {
	return subject(env, platform, "timewarp", "show")
}
func TimewarpHideSubject(env, platform string) string {
	return subject(env, platform, "timewarp", "hide")
}
func GPSShowSubject(env, platform string) string { return subject(env, platform, "gps", "show") }
func GPSHideSubject(env, platform string) string { return subject(env, platform, "gps", "hide") }

// LocationUpdateSubject carries the currently-playing clip's location + date
// from tripbot to onscreens-server, which caches it and feeds it into the
// rotators. On a bot-less YouTube stream the rotators surface this in place of
// the command hints — it's the info the !location / !date / !state commands
// would return, shown passively since no command can respond. Unlike the
// overlay show/hide commands, this is a periodic data feed, not a one-shot.
func LocationUpdateSubject(env, platform string) string {
	return subject(env, platform, "location", "update")
}
