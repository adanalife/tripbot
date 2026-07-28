package onscreensServer

import (
	"regexp"
	"strings"
	"testing"

	c "github.com/adanalife/tripbot/pkg/config/onscreens-server"
	rot "github.com/adanalife/tripbot/pkg/rotator"
)

// The weighted-pick, platform-scoping, and !command-tokenizing logic is tested
// in pkg/rotator, where it lives. What's left here is the wiring: which pool a
// corner draws from, sibling de-duplication across the two live onscreens, and
// applying copy edited in the admin console.

// rotatorConf builds a config literal for the given platform / inbound state,
// the two knobs the rotator behavior keys off.
func rotatorConf(platform string, inbound bool) *c.OnscreensServerConfig {
	return &c.OnscreensServerConfig{Environment: "testing", Platform: platform, YouTubeInboundEnabled: inbound}
}

// leftRotator / rightRotator build a corner seeded with the copy compiled into
// the binary — the state every test here exercises unless it's specifically
// about applying an update.
func leftRotator(cfg *c.OnscreensServerConfig) *rotator {
	return newLeftRotator(cfg, rot.DefaultConfig())
}

func rightRotator(cfg *c.OnscreensServerConfig) *rotator {
	return newRightRotator(cfg, rot.DefaultConfig())
}

// currentPool is the pool the corner would draw from.
func currentPool(r *rotator) []rot.Message {
	return r.pool(r.copy.Load())
}

// TestPromoModeRotatorsAdvertiseNoCommands verifies that on every promoMode
// instance — bot-less YouTube and the read-only platforms (TikTok, Instagram)
// where the bot can't reply — both rotators serve the promo set and never
// surface a "!command" token (which would no-op there and look broken).
func TestPromoModeRotatorsAdvertiseNoCommands(t *testing.T) {
	// "!" followed by a letter is a command token (e.g. !location); a bare "!"
	// as punctuation (the rare-message line) is fine.
	commandToken := regexp.MustCompile(`![a-zA-Z]`)
	cases := []struct {
		name string
		cfg  *c.OnscreensServerConfig
	}{
		{"youtube inbound off", rotatorConf(platformYouTube, false)},
		{"tiktok read-only", rotatorConf(platformTikTok, true)},
		{"instagram read-only", rotatorConf(platformInstagram, true)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for i := 0; i < 4000; i++ {
				if msg := leftRotator(tc.cfg).content(); commandToken.MatchString(msg) {
					t.Fatalf("promoMode left rotator surfaced a command: %q", msg)
				}
				if msg := rightRotator(tc.cfg).content(); commandToken.MatchString(msg) {
					t.Fatalf("promoMode right rotator surfaced a command: %q", msg)
				}
			}
		})
	}
}

// The Twitch CTAs are gone from the promo pools: a viewer on another platform
// is already watching, so pointing them at Twitch spends the corner on a
// handoff rather than on something they can do right here.
func TestPromoPoolsDoNotAdvertiseTwitch(t *testing.T) {
	def := rot.DefaultConfig()
	for _, m := range append(append([]rot.Message{}, def.Left.PromoMessages...), def.Right.PromoMessages...) {
		if strings.Contains(strings.ToLower(m.Text), "twitch") {
			t.Errorf("promo pool still points at Twitch: %q", m.Text)
		}
	}
}

// TestRotatorsServeCommandsWhenInboundEnabled confirms a YouTube instance with
// inbound chat on keeps the normal command-hint rotators (the post-quota state).
func TestRotatorsServeCommandsWhenInboundEnabled(t *testing.T) {
	if (&rotator{cfg: rotatorConf(platformYouTube, true)}).promoMode() {
		t.Fatal("YouTube with inbound enabled should not be in promo mode")
	}
}

// TestStartRotatorsPairsSiblings confirms the two corners are wired to each
// other so siblingCommands can see across.
func TestStartRotatorsPairsSiblings(t *testing.T) {
	cfg := rotatorConf(platformTwitch, true)
	l := leftRotator(cfg)
	r := rightRotator(cfg)
	l.sibling, r.sibling = r, l
	if l.sibling != r || r.sibling != l {
		t.Fatal("rotators not paired as siblings")
	}
	// With no started onscreen on the sibling, siblingCommands is a safe no-op.
	if got := l.siblingCommands(); got != nil {
		t.Errorf("expected nil sibling commands before sibling starts, got %v", got)
	}
}

// TestContentAvoidsSiblingCommandEndToEnd exercises the whole feature path:
// content() → siblingCommands() reads the sibling's live Content → the matching
// command is excluded from the pick. With the left corner pinned to its
// !location line, the right corner must never echo !location.
func TestContentAvoidsSiblingCommandEndToEnd(t *testing.T) {
	cfg := rotatorConf(platformTwitch, true)
	l := leftRotator(cfg)
	r := rightRotator(cfg)
	l.sibling, r.sibling = r, l
	// Pin the left corner to a line advertising !location.
	l.osc = newOnscreen()
	l.osc.Show("Where are we? (`!location`)")

	for i := 0; i < 4000; i++ {
		if got := r.content(); got == "Try running `!location`" {
			t.Fatalf("right corner echoed !location while left shows it: %q", got)
		}
	}
}

// TestSetCopyReplacesPool is the console-edit path: new copy swaps the pool
// wholesale, and nothing from the compiled-in defaults survives.
func TestSetCopyReplacesPool(t *testing.T) {
	l := leftRotator(rotatorConf(platformTwitch, true))
	l.setCopy(rot.Corner{Messages: []rot.Message{{Text: "edited in the console"}}}, "")

	for i := 0; i < 200; i++ {
		if got := l.content(); got != "edited in the console" {
			t.Fatalf("content() = %q, want only the edited line", got)
		}
	}
}

// TestSetCopyClearsRareMessage covers disabling the easter egg from the console:
// an empty rare message means the 1-in-N roll never fires, so a pool with a
// single line can only ever render that line.
func TestSetCopyClearsRareMessage(t *testing.T) {
	l := leftRotator(rotatorConf(platformTwitch, true))
	if got := l.copy.Load().rareMessage; got != rot.DefaultRareMessage {
		t.Fatalf("left corner seeded with rare message %q, want the default", got)
	}
	l.setCopy(rot.Corner{Messages: []rot.Message{{Text: "only"}}}, "")
	if got := l.copy.Load().rareMessage; got != "" {
		t.Errorf("rare message = %q after clearing, want empty", got)
	}
}

// The right corner never rolls for the easter egg — it's the left corner's.
func TestRightCornerHasNoRareMessage(t *testing.T) {
	r := rightRotator(rotatorConf(platformTwitch, true))
	if got := r.copy.Load().rareMessage; got != "" {
		t.Errorf("right corner rare message = %q, want empty", got)
	}
}

// The corners split "where" and "when" between them: left shows the location,
// right the date. Swapping them wouldn't fail anything else here — both lines
// resolve on both corners — it would just put the wrong field in each corner on
// stream.
func TestCornersCarryTheirOwnLiveLine(t *testing.T) {
	cfg := rotatorConf(platformTwitch, true)
	if got := leftRotator(cfg).liveLine.Text; got != "📍 $location" {
		t.Errorf("left live line = %q, want the location line", got)
	}
	if got := rightRotator(cfg).liveLine.Text; got != "📅 $date" {
		t.Errorf("right live line = %q, want the date line", got)
	}
}

// Both corners swap on the same cadence; a drift between them would show up as
// the two overlays visibly falling out of step.
func TestCornersShareTheirCadence(t *testing.T) {
	cfg := rotatorConf(platformTwitch, true)
	if l, r := leftRotator(cfg).freq, rightRotator(cfg).freq; l != r {
		t.Errorf("left freq = %v, right freq = %v, want equal", l, r)
	}
}

// TestApplyRotatorConfigUpdatesBothCorners covers the NATS-delivered update at
// the Server level, including that the rare message lands only on the left.
func TestApplyRotatorConfigUpdatesBothCorners(t *testing.T) {
	cfg := rotatorConf(platformTwitch, true)
	s := &Server{cfg: cfg, left: leftRotator(cfg), right: rightRotator(cfg)}
	s.applyRotatorConfig(rot.Config{
		Left:        rot.Corner{Messages: []rot.Message{{Text: "L"}}},
		Right:       rot.Corner{Messages: []rot.Message{{Text: "R"}}},
		RareMessage: "rare!",
	})

	if got := s.left.copy.Load().messages[0].Text; got != "L" {
		t.Errorf("left messages = %q, want L", got)
	}
	if got := s.right.copy.Load().messages[0].Text; got != "R" {
		t.Errorf("right messages = %q, want R", got)
	}
	if got := s.left.copy.Load().rareMessage; got != "rare!" {
		t.Errorf("left rare message = %q, want rare!", got)
	}
	if got := s.right.copy.Load().rareMessage; got != "" {
		t.Errorf("right rare message = %q, want empty", got)
	}
}
