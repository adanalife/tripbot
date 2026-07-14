package vlcEvents

import (
	"fmt"
	"strings"
)

// domain is the fixed segment between <env> and the verb in every vlc
// subject: tripbot.<env>.vlc.<verb>.<platform>.
const domain = "vlc"

// subject builds tripbot.<env>.vlc.<verb...>. Unexported so callers go
// through the typed constructors below — that keeps the verb strings in one
// place and out of reach of typos at the call site. The variadic tail lets
// the two-segment play.random / play.file group alongside the flat skip /
// back.
func subject(env string, verb ...string) string {
	return fmt.Sprintf("tripbot.%s.%s.%s", env, domain, strings.Join(verb, "."))
}

// The command subjects carry a trailing platform leaf so each streaming
// platform's playback stays isolated: vlc-twitch and vlc-youtube share the
// env's NATS and each subscribes only to its own leaf, so a Twitch-triggered
// !find never seeks the YouTube stream — same shape as the lastplayed leaf below.
func PlayRandomSubject(env, platform string) string { return subject(env, "play", "random", platform) }
func PlayFileSubject(env, platform string) string   { return subject(env, "play", "file", platform) }
func PlayFileAtSubject(env, platform string) string { return subject(env, "play", "at", platform) }
func SkipSubject(env, platform string) string       { return subject(env, "skip", platform) }
func BackSubject(env, platform string) string       { return subject(env, "back", platform) }

// LastPlayedSubject is the per-platform leaf vlc-server publishes its
// now-playing state to (tripbot.<env>.vlc.lastplayed.<platform>). Unlike the
// command subjects above this one is *state*, not a command: every platform
// instance (vlc-twitch, vlc-youtube) shares the env's NATS, so the platform
// leaf keeps the TRIPBOT_VLC_LASTPLAYED stream's last-value cache
// (MaxMsgsPerSubject=1) per instance instead of the instances clobbering one
// another — same shape as tripbot.<env>.auth.status.<platform>.
func LastPlayedSubject(env, platform string) string {
	return subject(env, "lastplayed") + "." + platform
}

// LastPlayedWildcard covers every platform's lastplayed leaf in env — the
// subject filter the TRIPBOT_VLC_LASTPLAYED stream is declared with.
func LastPlayedWildcard(env string) string { return subject(env, "lastplayed") + ".*" }
