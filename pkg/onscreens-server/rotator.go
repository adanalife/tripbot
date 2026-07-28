package onscreensServer

import (
	"log/slog"
	"math/rand"
	"sync/atomic"
	"time"

	c "github.com/adanalife/tripbot/pkg/config/onscreens-server"
	rot "github.com/adanalife/tripbot/pkg/rotator"
)

// The message type, platform constants, default copy, weighted-pick logic, and
// per-corner render budgets all live in pkg/rotator, shared with the tripbot
// /api/rotators surface that serves and stores console edits. That package is
// stdlib-only, so importing it here doesn't drag config/DB init into this binary
// (the package-boundary-init-discipline ADR).
const (
	platformTwitch    = rot.PlatformTwitch
	platformYouTube   = rot.PlatformYouTube
	platformTikTok    = rot.PlatformTikTok
	platformInstagram = rot.PlatformInstagram
)

// rotatorCopy is the editable copy one corner is currently rotating through.
// Held behind a single atomic pointer so a console edit swaps all three fields
// at once — the render loop can't observe a half-applied update (new messages
// against a stale rare line) and doesn't need a lock on the read path.
type rotatorCopy struct {
	messages      []rot.Message // full pool (every command hint)
	promoMessages []rot.Message // promoMode pool (no reply-only command hints)
	rareMessage   string        // 1-in-rot.RareOdds easter egg; "" = none
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
	cfg      *c.OnscreensServerConfig
	kind     string        // for logs: "left-rotator" / "right-rotator"
	freq     time.Duration // how often the visible line swaps
	liveLine rot.Message   // promoMode live-data line (location/date); zero = none

	// copy is swapped wholesale when edited copy arrives from the admin console;
	// the loop goroutine only ever loads it. Never nil once the constructor ran.
	copy atomic.Pointer[rotatorCopy]

	osc     *Onscreen // render target; nil until start()
	sibling *rotator  // the other corner, for command de-duplication
}

// startRotators builds both corner rotators from the copy compiled into the
// binary, pairs them as siblings (so neither advertises a command the other is
// currently showing), and starts their background loops. Copy edited in the
// admin console is applied afterwards by the caller (RestoreRotatorCopy), so the
// overlays render valid defaults during the window before NATS is up.
//
// Left is started first, so it primes with no sibling content yet (right.osc is
// still nil — siblingCommands no-ops); right then primes against left's first
// line.
func startRotators(cfg *c.OnscreensServerConfig) (left, right *rotator) {
	def := rot.DefaultConfig()
	l := newLeftRotator(cfg, def)
	r := newRightRotator(cfg, def)
	l.sibling, r.sibling = r, l
	l.start()
	r.start()
	return l, r
}

// setCopy swaps in new copy for this corner. The next rotation tick renders it;
// whatever line is on screen right now is left alone rather than yanked
// mid-display.
func (r *rotator) setCopy(corner rot.Corner, rareMessage string) {
	r.copy.Store(&rotatorCopy{
		messages:      corner.Messages,
		promoMessages: corner.PromoMessages,
		rareMessage:   rareMessage,
	})
}

// applyRotatorConfig swaps new copy into both corners. The rare message is the
// left corner's alone; the right corner never rolls for it.
func (s *Server) applyRotatorConfig(cfg rot.Config) {
	s.left.setCopy(cfg.Left, cfg.RareMessage)
	s.right.setCopy(cfg.Right, "")
}

// start creates the rotator's *Onscreen, primes it with a first message
// synchronously (so the OBS browser source has content to render the moment it
// polls — otherwise there's a brief race where the rotator is empty until the
// goroutine schedules), and kicks off the background rotation loop.
func (r *rotator) start() {
	slog.Info("creating onscreen", "kind", r.kind)
	r.osc = newOnscreen()
	r.osc.Show(r.content())
	go r.loop()
}

func (r *rotator) loop() {
	for { // forever
		time.Sleep(r.freq)
		r.osc.Show(r.content())
	}
}

// content picks the next line to display: the rare easter egg on a lucky roll,
// otherwise a weighted-random pick from this corner's pool that doesn't collide
// with whatever command the sibling corner is currently showing. Either way the
// line's $variables are substituted from the clip data tripbot last pushed.
//
// A rare line whose variables don't resolve falls through to the pool rather than
// spending the 1-in-RareOdds roll on a line it can't render.
func (r *rotator) content() string {
	cp := r.copy.Load()
	now := time.Now()
	vars := liveLocation.snapshot(now)
	if cp.rareMessage != "" && vars.Resolvable(cp.rareMessage) && rand.Intn(rot.RareOdds) == 0 {
		return vars.Expand(cp.rareMessage)
	}
	return rot.Pick(r.cfg.Platform, r.pool(cp), r.siblingCommands(), vars)
}

// pool returns the message set for the current instance state: the promo pool
// plus this corner's live-data line on an instance that can't surface a command
// result, otherwise the full command-hint pool. The live line is appended
// unconditionally — it's written in $variables like any other line, so Pick drops
// it on its own whenever the clip data is missing or stale.
func (r *rotator) pool(cp *rotatorCopy) []rot.Message {
	if !r.promoMode() {
		return cp.messages
	}
	if r.liveLine.Text == "" {
		return cp.promoMessages
	}
	return append([]rot.Message{r.liveLine}, cp.promoMessages...)
}

// siblingCommands is the set of !command tokens the other corner is currently
// showing, used to keep both corners from advertising the same command at once.
func (r *rotator) siblingCommands() map[string]bool {
	if r.sibling == nil || r.sibling.osc == nil {
		return nil
	}
	return rot.CommandsIn(r.sibling.osc.Content)
}

// promoMode reports whether this corner draws from the promo pool instead of
// the command-hint pool — true whenever a hinted "!command" couldn't produce a
// result the viewer sees, which would read as broken. Two cases:
//
//   - a bot-less YouTube instance (YOUTUBE_INBOUND_ENABLED=false) receives no
//     commands at all;
//   - a read-only platform (TikTok, Instagram) receives commands but the bot
//     can't post a reply (neither has a chat-send API — the gateway
//     webcast/poll is observe-only), so a command whose result is a chat line
//     looks unanswered.
//
// Mirrors the chatbot's command gating (v1Commands + the read-only platforms).
// Twitch and an inbound-on YouTube run the command-hint pool.
func (r *rotator) promoMode() bool {
	switch r.cfg.Platform {
	case platformTikTok, platformInstagram:
		return true
	case platformYouTube:
		return !r.cfg.YouTubeInboundEnabled
	default:
		return false
	}
}
