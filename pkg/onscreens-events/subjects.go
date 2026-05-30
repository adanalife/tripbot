package onscreensEvents

import "fmt"

// domain is the fixed segment between <env> and <overlay> in every
// onscreens subject: tripbot.<env>.onscreens.<overlay>.<verb>.
const domain = "onscreens"

// subject builds tripbot.<env>.onscreens.<overlay>.<verb>. Unexported so
// callers go through the typed constructors below — that keeps the
// overlay/verb strings in one place and out of reach of typos at the call
// site.
func subject(env, overlay, verb string) string {
	return fmt.Sprintf("tripbot.%s.%s.%s.%s", env, domain, overlay, verb)
}

func MiddleShowSubject(env string) string      { return subject(env, "middle", "show") }
func MiddleHideSubject(env string) string      { return subject(env, "middle", "hide") }
func LeaderboardShowSubject(env string) string { return subject(env, "leaderboard", "show") }
func LeaderboardHideSubject(env string) string { return subject(env, "leaderboard", "hide") }
func TimewarpShowSubject(env string) string    { return subject(env, "timewarp", "show") }
func TimewarpHideSubject(env string) string    { return subject(env, "timewarp", "hide") }
func GPSShowSubject(env string) string         { return subject(env, "gps", "show") }
func GPSHideSubject(env string) string         { return subject(env, "gps", "hide") }

// FlagHideSubject is the only flag subject. flag.show is intentionally
// absent — the feature is disabled (the HTTP route 501s and the client
// method is a no-op), so publishing it would be dead surface.
func FlagHideSubject(env string) string { return subject(env, "flag", "hide") }
