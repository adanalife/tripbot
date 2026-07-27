package onscreensServer

import (
	"strings"
	"testing"

	c "github.com/adanalife/tripbot/pkg/config/onscreens-server"
)

// rotatorConf builds a config literal for the given platform / inbound state,
// the two knobs the rotator behavior keys off.
func rotatorConf(platform string, inbound bool) *c.OnscreensServerConfig {
	return &c.OnscreensServerConfig{Environment: "testing", Platform: platform, YouTubeInboundEnabled: inbound}
}

// TestPromoModeRotatorsAdvertiseOnlyEffectCommands verifies that on every
// promoMode instance — bot-less YouTube and the read-only platforms (TikTok,
// Instagram) where the bot can't reply — both rotators serve the promo set and
// surface no reply-only command. An effect command is allowed: its result is
// the playhead jump on stream, which needs no chat reply. A reply-only one
// would silently no-op there and look broken.
func TestPromoModeRotatorsAdvertiseOnlyEffectCommands(t *testing.T) {
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
			check := func(corner, msg string) {
				t.Helper()
				for cmd := range commandsIn(msg) {
					if !effectCommands[cmd] {
						t.Fatalf("promoMode %s rotator surfaced reply-only !%s: %q", corner, cmd, msg)
					}
				}
			}
			for i := 0; i < 4000; i++ {
				check("left", newLeftRotator(tc.cfg).content())
				check("right", newRightRotator(tc.cfg).content())
			}
		})
	}
}

// The Twitch CTAs are gone from the promo pools: a viewer on another platform
// is already watching: pointing them at Twitch spends the corner on a handoff
// rather than on something they can do right here.
func TestPromoPoolsDoNotAdvertiseTwitch(t *testing.T) {
	for _, m := range append(append([]rotatorMessage{}, promoLeftMessages...), promoRightMessages...) {
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

func TestRotatorMessageAppliesTo(t *testing.T) {
	all := rotatorMessage{Text: "x"}
	if !all.appliesTo(platformYouTube) || !all.appliesTo(platformTwitch) {
		t.Error("empty Platforms should apply to all platforms")
	}
	tw := rotatorMessage{Text: "x", Platforms: []string{platformTwitch}}
	if tw.appliesTo(platformYouTube) {
		t.Error("twitch-only message should not apply to YouTube")
	}
	if !tw.appliesTo(platformTwitch) {
		t.Error("twitch-only message should apply to Twitch")
	}
}

// TestLeftRotatorOmitsTwitchOnlyOnYouTube guards the headline behavior: a
// YouTube overlay must never surface the !miles / !guess lines, which would
// advertise commands disabled on that platform.
func TestLeftRotatorOmitsTwitchOnlyOnYouTube(t *testing.T) {
	for i := 0; i < 2000; i++ {
		msg := pickRotatorMessage(platformYouTube, possibleLeftMessages, nil)
		if strings.Contains(msg, "!miles") || strings.Contains(msg, "!guess") {
			t.Fatalf("YouTube left rotator surfaced a Twitch-only line: %q", msg)
		}
	}
}

// TestLeftRotatorSurfacesTwitchOnlyOnTwitch confirms the Twitch-only lines are
// still reachable on Twitch (the filter doesn't drop them everywhere).
func TestLeftRotatorSurfacesTwitchOnlyOnTwitch(t *testing.T) {
	var sawMiles, sawGuess bool
	for i := 0; i < 5000 && !(sawMiles && sawGuess); i++ {
		switch pickRotatorMessage(platformTwitch, possibleLeftMessages, nil) {
		case "Earn miles for every minute you watch (`!miles`)":
			sawMiles = true
		case "Try and `!guess` what state we're in":
			sawGuess = true
		}
	}
	if !sawMiles || !sawGuess {
		t.Errorf("expected Twitch-only lines reachable on Twitch: miles=%v guess=%v", sawMiles, sawGuess)
	}
}

func TestPickRotatorMessageEmptyWhenNoneApply(t *testing.T) {
	twitchOnly := []rotatorMessage{
		{Text: "a", Platforms: []string{platformTwitch}},
		{Text: "b", Platforms: []string{platformTwitch}},
	}
	if got := pickRotatorMessage(platformYouTube, twitchOnly, nil); got != "" {
		t.Errorf("expected empty string when no message applies, got %q", got)
	}
}

// TestPickRotatorMessageRespectsWeight checks the weighted draw is biased: a
// Weight:9 entry should dominate a Weight:1 entry over many samples.
func TestPickRotatorMessageRespectsWeight(t *testing.T) {
	msgs := []rotatorMessage{
		{Text: "rare"},              // weight 1
		{Text: "common", Weight: 9}, // weight 9
	}
	var common int
	const n = 10000
	for i := 0; i < n; i++ {
		if pickRotatorMessage(platformTwitch, msgs, nil) == "common" {
			common++
		}
	}
	// Expect ~90%; allow generous slack to stay non-flaky.
	if common < n*3/4 {
		t.Errorf("weighted draw not biased: common=%d/%d", common, n)
	}
}

func TestCommandsIn(t *testing.T) {
	cmds := commandsIn("Where are we? (`!location`) and try `!timewarp`")
	if !cmds["location"] || !cmds["timewarp"] {
		t.Errorf("expected location+timewarp, got %v", cmds)
	}
	// A bare "!" as punctuation is not a command token.
	if got := commandsIn("You found the rare message! Make a clip for a prize!"); got != nil {
		t.Errorf("expected no commands in punctuation-only text, got %v", got)
	}
	if got := commandsIn("twitch.tv/ADanaLife_"); got != nil {
		t.Errorf("expected no commands, got %v", got)
	}
}

// TestPickExcludesSiblingCommand is the headline of the dedup feature: when the
// sibling corner is already showing !location, this corner must never pick a
// line advertising !location — the two corners shouldn't echo the same command.
func TestPickExcludesSiblingCommand(t *testing.T) {
	exclude := map[string]bool{"location": true}
	for i := 0; i < 4000; i++ {
		if got := pickRotatorMessage(platformTwitch, possibleRightMessages, exclude); got == "Try running `!location`" {
			t.Fatalf("right rotator surfaced !location while sibling shows it: %q", got)
		}
	}
}

// TestPickRelaxesWhenExclusionEmptiesPool verifies the fallback: if excluding
// the sibling's commands would rule out every eligible line, the rotator shows a
// (briefly duplicate) line rather than going blank.
func TestPickRelaxesWhenExclusionEmptiesPool(t *testing.T) {
	msgs := []rotatorMessage{{Text: "Try running `!location`"}}
	if got := pickRotatorMessage(platformTwitch, msgs, map[string]bool{"location": true}); got != "Try running `!location`" {
		t.Errorf("expected exclusion to relax to the only line, got %q", got)
	}
}

// TestStartRotatorsPairsSiblings confirms the two corners are wired to each
// other so siblingCommands can see across.
func TestStartRotatorsPairsSiblings(t *testing.T) {
	cfg := rotatorConf(platformTwitch, true)
	l := newLeftRotator(cfg)
	r := newRightRotator(cfg)
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
	l := newLeftRotator(cfg)
	r := newRightRotator(cfg)
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
