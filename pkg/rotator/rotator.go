// Package rotator holds the wire format, default copy, and selection logic for
// the corner overlay rotators — the two bottom-strip message boxes that cycle
// hints and promo lines over the dashcam video.
//
// It is deliberately dependency-free (stdlib only): both onscreens-server (which
// renders the rotators) and tripbot's /api/rotators surface (which serves and
// stores edited copy for the admin console) import it. Pulling any
// pkg/config/<binary> or pkg/database import in here would drag DB/config init
// into onscreens-server's binary — the failure mode behind the v2.17.0
// vlc-server outage. The guard is
// `go list -deps ./cmd/onscreens-server | grep tripbot/pkg/`.
package rotator

import (
	"math/rand"
	"regexp"
	"slices"
)

// Streaming platforms a message can be scoped to. Mirrors the values pkg/chatbot
// uses, but kept local — importing chatbot from here would defeat the whole
// point of this package.
const (
	PlatformTwitch    = "twitch"
	PlatformYouTube   = "youtube"
	PlatformTikTok    = "tiktok"
	PlatformInstagram = "instagram"
	PlatformFacebook  = "facebook"
)

// RareOdds is the 1-in-N chance the left rotator shows its easter-egg line.
const RareOdds = 10000

// Message is one line a rotator can display.
//
//   - Platforms scopes the line to specific streaming platforms; empty means
//     "all platforms". This is what keeps a YouTube overlay from advertising
//     Twitch-only commands (!miles, !guess). Platform-scoping only applies to
//     the defaults below — copy edited in the admin console is stored per
//     platform already, so an edited pool carries no Platforms at all.
//   - Weight biases weighted-random selection (<1 is treated as 1), making a
//     message proportionally more frequent without listing it twice.
type Message struct {
	Text      string   `json:"text"`
	Platforms []string `json:"platforms,omitempty"`
	Weight    int      `json:"weight,omitempty"`
}

// AppliesTo reports whether m should show on the given platform.
func (m Message) AppliesTo(platform string) bool {
	return len(m.Platforms) == 0 || slices.Contains(m.Platforms, platform)
}

// Weighted returns m's selection weight, treating anything under 1 as 1 so an
// unset Weight behaves as "normal frequency".
func (m Message) Weighted() int {
	if m.Weight < 1 {
		return 1
	}
	return m.Weight
}

// commandTokenRE matches a "!command" token: a bang followed by word chars (so a
// bare "!" used as punctuation, e.g. in the rare-message line, isn't a command).
var commandTokenRE = regexp.MustCompile(`!(\w+)`)

// CommandsIn returns the set of !command tokens mentioned in text (without the
// leading bang). Empty/nil when text advertises no commands.
func CommandsIn(text string) map[string]bool {
	matches := commandTokenRE.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return nil
	}
	cmds := make(map[string]bool, len(matches))
	for _, m := range matches {
		cmds[m[1]] = true
	}
	return cmds
}

// SharesCommand reports whether m advertises any command in exclude.
func (m Message) SharesCommand(exclude map[string]bool) bool {
	if len(exclude) == 0 {
		return false
	}
	for cmd := range CommandsIn(m.Text) {
		if exclude[cmd] {
			return true
		}
	}
	return false
}

// Pick returns a weighted-random message among those applicable to platform,
// not advertising a command in exclude (the sibling corner's current commands),
// and whose $variables all resolve against vars — with those variables
// substituted. Returns "" when no message applies at all.
//
// If exclude rules out every otherwise-eligible message, the exclusion is relaxed
// rather than showing nothing — better a brief duplicate than a blank corner. The
// variable check is never relaxed: a line whose data hasn't arrived would go to
// air with a literal "$weather" in it, which is worse than the corner skipping it
// this rotation.
func Pick(platform string, msgs []Message, exclude map[string]bool, vars Vars) string {
	eligible := func(m Message) bool {
		return m.AppliesTo(platform) && !m.SharesCommand(exclude) && vars.Resolvable(m.Text)
	}

	total := 0
	for _, m := range msgs {
		if eligible(m) {
			total += m.Weighted()
		}
	}
	if total == 0 {
		if len(exclude) > 0 {
			// Every candidate collided with the sibling; relax and retry once.
			return Pick(platform, msgs, nil, vars)
		}
		return ""
	}

	n := rand.Intn(total)
	for _, m := range msgs {
		if !eligible(m) {
			continue
		}
		if n -= m.Weighted(); n < 0 {
			return vars.Expand(m.Text)
		}
	}
	return "" // unreachable: n < total guarantees a hit above
}
