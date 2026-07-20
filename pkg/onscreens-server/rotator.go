package onscreensServer

import (
	"log/slog"
	"math/rand"
	"regexp"
	"slices"
	"time"

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

// rareOdds is the 1-in-N chance the left rotator shows its easter-egg line.
const rareOdds = 10000

// rotatorMessage is one line a rotator can display.
//
//   - Platforms scopes the line to specific streaming platforms; empty means
//     "all platforms". This is what keeps a YouTube overlay from advertising
//     Twitch-only commands (!miles, !guess).
//   - Weight biases weighted-random selection (<1 is treated as 1), making a
//     message proportionally more frequent without listing it twice.
//
// The hardcoded slices are the current source of truth; this struct is also the
// seam for a future admin-console-editable source (swap the slice for a loaded
// one) — tracked as a TODO, not built yet.
type rotatorMessage struct {
	Text      string
	Platforms []string
	Weight    int
}

// rotator drives one corner overlay (left or right). Both corners are the same
// machine — a weighted-random pool swapped on a fixed cadence — differing only
// in their message data, cadence, optional live-data line, and easter egg. The
// per-corner specifics are wired in newLeftRotator / newRightRotator.
//
// sibling is the other corner. When set, a rotator avoids picking a line that
// advertises a command the sibling is currently showing, so the two corners
// don't both say "!location" at once (which reads as broken).
type rotator struct {
	cfg             *c.OnscreensServerConfig
	kind            string                                     // for logs: "left-rotator" / "right-rotator"
	freq            time.Duration                              // how often the visible line swaps
	messages        []rotatorMessage                           // bot-enabled pool (command hints)
	botlessMessages []rotatorMessage                           // bot-less promo pool (no commands)
	liveLine        func(now time.Time) (rotatorMessage, bool) // bot-less live-data line (location/date); nil = none
	rareMessage     string                                     // 1-in-rareOdds easter egg; "" = none

	osc     *Onscreen // render target; nil until start()
	sibling *rotator  // the other corner, for command de-duplication
}

// startRotators builds both corner rotators, pairs them as siblings (so neither
// advertises a command the other is currently showing), starts their background
// loops, and returns their *Onscreen render targets. Left is started first, so
// it primes with no sibling content yet (right.osc is still nil — siblingCommands
// no-ops); right then primes against left's first line.
func startRotators(cfg *c.OnscreensServerConfig) (left, right *Onscreen) {
	l := newLeftRotator(cfg)
	r := newRightRotator(cfg)
	l.sibling, r.sibling = r, l
	return l.start(), r.start()
}

// start creates the rotator's *Onscreen, primes it with a first message
// synchronously (so the OBS browser source has content to render the moment it
// polls — otherwise there's a brief race where the rotator is empty until the
// goroutine schedules), kicks off the background rotation loop, and returns the
// onscreen.
func (r *rotator) start() *Onscreen {
	slog.Info("creating onscreen", "kind", r.kind)
	r.osc = newOnscreen()
	r.osc.Show(r.content())
	go r.loop()
	return r.osc
}

func (r *rotator) loop() {
	for { // forever
		time.Sleep(r.freq)
		r.osc.Show(r.content())
	}
}

// content picks the next line to display: the rare easter egg on a lucky roll,
// otherwise a weighted-random pick from this corner's pool that doesn't collide
// with whatever command the sibling corner is currently showing.
func (r *rotator) content() string {
	if r.rareMessage != "" && rand.Intn(rareOdds) == 0 {
		return r.rareMessage
	}
	return pickRotatorMessage(r.cfg.Platform, r.pool(time.Now()), r.siblingCommands())
}

// pool returns the message set for the current instance state: the bot-less
// promo pool (with the live location/date line prepended when fresh) on a
// bot-less YouTube instance, otherwise the normal command-hint pool.
func (r *rotator) pool(now time.Time) []rotatorMessage {
	if !r.botless() {
		return r.messages
	}
	if r.liveLine != nil {
		if line, ok := r.liveLine(now); ok {
			return append([]rotatorMessage{line}, r.botlessMessages...)
		}
	}
	return r.botlessMessages
}

// siblingCommands is the set of !command tokens the other corner is currently
// showing, used to keep both corners from advertising the same command at once.
func (r *rotator) siblingCommands() map[string]bool {
	if r.sibling == nil || r.sibling.osc == nil {
		return nil
	}
	return commandsIn(r.sibling.osc.Content)
}

// botless reports whether this instance should show promotional copy instead of
// command hints — true only for a YouTube pipeline whose inbound chat is off
// (YOUTUBE_INBOUND_ENABLED=false). In that state no command can respond, so the
// rotators must not advertise commands. Mirrors the chatbot's botless gate.
func (r *rotator) botless() bool {
	return r.cfg.Platform == platformYouTube && !r.cfg.YouTubeInboundEnabled
}

// appliesTo reports whether m should show on the given platform.
func (m rotatorMessage) appliesTo(platform string) bool {
	return len(m.Platforms) == 0 || slices.Contains(m.Platforms, platform)
}

func (m rotatorMessage) weight() int {
	if m.Weight < 1 {
		return 1
	}
	return m.Weight
}

// commandTokenRE matches a "!command" token: a bang followed by word chars (so a
// bare "!" used as punctuation, e.g. in the rare-message line, isn't a command).
var commandTokenRE = regexp.MustCompile(`!(\w+)`)

// commandsIn returns the set of !command tokens mentioned in text (without the
// leading bang). Empty/nil when text advertises no commands.
func commandsIn(text string) map[string]bool {
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

// sharesCommand reports whether m advertises any command in exclude.
func (m rotatorMessage) sharesCommand(exclude map[string]bool) bool {
	if len(exclude) == 0 {
		return false
	}
	for cmd := range commandsIn(m.Text) {
		if exclude[cmd] {
			return true
		}
	}
	return false
}

// pickRotatorMessage returns a weighted-random message among those applicable to
// this instance's platform and not advertising a command in exclude (the
// sibling corner's current commands). Returns "" when no message applies at all.
// If exclude rules out every otherwise-eligible message, the exclusion is
// relaxed rather than showing nothing — better a brief duplicate than a blank
// corner.
func pickRotatorMessage(platform string, msgs []rotatorMessage, exclude map[string]bool) string {
	eligible := func(m rotatorMessage) bool {
		return m.appliesTo(platform) && !m.sharesCommand(exclude)
	}

	total := 0
	for _, m := range msgs {
		if eligible(m) {
			total += m.weight()
		}
	}
	if total == 0 {
		if len(exclude) > 0 {
			// Every candidate collided with the sibling; relax and retry once.
			return pickRotatorMessage(platform, msgs, nil)
		}
		return ""
	}

	n := rand.Intn(total)
	for _, m := range msgs {
		if !eligible(m) {
			continue
		}
		if n -= m.weight(); n < 0 {
			return m.Text
		}
	}
	return "" // unreachable: n < total guarantees a hit above
}
