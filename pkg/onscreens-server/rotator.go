package onscreensServer

import (
	"math/rand"

	c "github.com/adanalife/tripbot/pkg/config/onscreens-server"
)

// Streaming platforms a rotator message can be scoped to. Mirrors the values
// pkg/chatbot uses, but kept local — onscreens-server must not import chatbot
// (that would drag tripbot-only config/DB init into this binary; see the
// package-boundary-init-discipline ADR).
const (
	platformTwitch  = "twitch"
	platformYouTube = "youtube"
)

// rotatorMessage is one line a rotator can display.
//
//   - Platforms scopes the line to specific streaming platforms; empty means
//     "all platforms". This is what keeps a YouTube overlay from advertising
//     Twitch-only commands (!miles, !guess).
//   - Weight biases weighted-random selection (<1 is treated as 1). It replaces
//     the old "list the same line twice" trick for making a message more
//     frequent.
//
// The hardcoded slices are the current source of truth; this struct is also the
// seam for a future admin-console-editable source (swap the slice for a loaded
// one) — tracked as a TODO, not built yet.
type rotatorMessage struct {
	Text      string
	Platforms []string
	Weight    int
}

// botless reports whether this instance should show promotional copy instead of
// command hints — true only for a YouTube pipeline whose inbound chat is off
// (YOUTUBE_INBOUND_ENABLED=false). In that state no command can respond, so the
// rotators must not advertise commands. Mirrors the chatbot's botless gate.
func botless() bool {
	return c.Conf.Platform == platformYouTube && !c.Conf.YouTubeInboundEnabled
}

// appliesTo reports whether m should show on the given platform.
func (m rotatorMessage) appliesTo(platform string) bool {
	if len(m.Platforms) == 0 {
		return true
	}
	for _, p := range m.Platforms {
		if p == platform {
			return true
		}
	}
	return false
}

func (m rotatorMessage) weight() int {
	if m.Weight < 1 {
		return 1
	}
	return m.Weight
}

// pickRotatorMessage returns a weighted-random message among those applicable
// to this instance's platform. Returns "" when none apply (the rotator shows
// nothing rather than panicking).
func pickRotatorMessage(msgs []rotatorMessage) string {
	platform := c.Conf.Platform

	total := 0
	for _, m := range msgs {
		if m.appliesTo(platform) {
			total += m.weight()
		}
	}
	if total == 0 {
		return ""
	}

	n := rand.Intn(total)
	for _, m := range msgs {
		if !m.appliesTo(platform) {
			continue
		}
		if n -= m.weight(); n < 0 {
			return m.Text
		}
	}
	return "" // unreachable: n < total guarantees a hit above
}
