package vlcEvents

import (
	"fmt"
	"strings"
)

// domain is the fixed segment between <env> and the verb in every vlc
// subject: tripbot.<env>.vlc.<verb>.
const domain = "vlc"

// subject builds tripbot.<env>.vlc.<verb...>. Unexported so callers go
// through the typed constructors below — that keeps the verb strings in one
// place and out of reach of typos at the call site. The variadic tail lets
// the two-segment play.random / play.file group alongside the flat skip /
// back.
func subject(env string, verb ...string) string {
	return fmt.Sprintf("tripbot.%s.%s.%s", env, domain, strings.Join(verb, "."))
}

func PlayRandomSubject(env string) string { return subject(env, "play", "random") }
func PlayFileSubject(env string) string   { return subject(env, "play", "file") }
func SkipSubject(env string) string       { return subject(env, "skip") }
func BackSubject(env string) string       { return subject(env, "back") }
