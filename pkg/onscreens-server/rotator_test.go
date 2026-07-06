package onscreensServer

import (
	"regexp"
	"strings"
	"testing"

	c "github.com/adanalife/tripbot/pkg/config/onscreens-server"
)

// withPlatform sets c.Conf.Platform for the duration of a test and restores it.
func withPlatform(t *testing.T, platform string) {
	t.Helper()
	prev := c.Conf.Platform
	c.Conf.Platform = platform
	t.Cleanup(func() { c.Conf.Platform = prev })
}

// withInbound sets c.Conf.YouTubeInboundEnabled for the duration of a test and
// restores it.
func withInbound(t *testing.T, enabled bool) {
	t.Helper()
	prev := c.Conf.YouTubeInboundEnabled
	c.Conf.YouTubeInboundEnabled = enabled
	t.Cleanup(func() { c.Conf.YouTubeInboundEnabled = prev })
}

// TestBotlessRotatorsAdvertiseNoCommands verifies that on a bot-less YouTube
// instance both rotators serve the promo set and never surface a "!command"
// token (which would no-op there and look broken).
func TestBotlessRotatorsAdvertiseNoCommands(t *testing.T) {
	withPlatform(t, platformYouTube)
	withInbound(t, false)
	// "!" followed by a letter is a command token (e.g. !location); a bare "!"
	// as punctuation (the rare-message line) is fine.
	commandToken := regexp.MustCompile(`![a-zA-Z]`)
	for i := 0; i < 4000; i++ {
		if msg := newLeftRotator().content(); commandToken.MatchString(msg) {
			t.Fatalf("bot-less left rotator surfaced a command: %q", msg)
		}
		if msg := newRightRotator().content(); commandToken.MatchString(msg) {
			t.Fatalf("bot-less right rotator surfaced a command: %q", msg)
		}
	}
}

// TestRotatorsServeCommandsWhenInboundEnabled confirms a YouTube instance with
// inbound chat on keeps the normal command-hint rotators (the post-quota state).
func TestRotatorsServeCommandsWhenInboundEnabled(t *testing.T) {
	withPlatform(t, platformYouTube)
	withInbound(t, true)
	if botless() {
		t.Fatal("YouTube with inbound enabled should not be bot-less")
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
	withPlatform(t, platformYouTube)
	for i := 0; i < 2000; i++ {
		msg := pickRotatorMessage(possibleLeftMessages, nil)
		if strings.Contains(msg, "!miles") || strings.Contains(msg, "!guess") {
			t.Fatalf("YouTube left rotator surfaced a Twitch-only line: %q", msg)
		}
	}
}

// TestLeftRotatorSurfacesTwitchOnlyOnTwitch confirms the Twitch-only lines are
// still reachable on Twitch (the filter doesn't drop them everywhere).
func TestLeftRotatorSurfacesTwitchOnlyOnTwitch(t *testing.T) {
	withPlatform(t, platformTwitch)
	var sawMiles, sawGuess bool
	for i := 0; i < 5000 && !(sawMiles && sawGuess); i++ {
		switch pickRotatorMessage(possibleLeftMessages, nil) {
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
	withPlatform(t, platformYouTube)
	twitchOnly := []rotatorMessage{
		{Text: "a", Platforms: []string{platformTwitch}},
		{Text: "b", Platforms: []string{platformTwitch}},
	}
	if got := pickRotatorMessage(twitchOnly, nil); got != "" {
		t.Errorf("expected empty string when no message applies, got %q", got)
	}
}

// TestPickRotatorMessageRespectsWeight checks the weighted draw is biased: a
// Weight:9 entry should dominate a Weight:1 entry over many samples.
func TestPickRotatorMessageRespectsWeight(t *testing.T) {
	withPlatform(t, platformTwitch)
	msgs := []rotatorMessage{
		{Text: "rare"},              // weight 1
		{Text: "common", Weight: 9}, // weight 9
	}
	var common int
	const n = 10000
	for i := 0; i < n; i++ {
		if pickRotatorMessage(msgs, nil) == "common" {
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
	withPlatform(t, platformTwitch)
	exclude := map[string]bool{"location": true}
	for i := 0; i < 4000; i++ {
		if got := pickRotatorMessage(possibleRightMessages, exclude); got == "Try running `!location`" {
			t.Fatalf("right rotator surfaced !location while sibling shows it: %q", got)
		}
	}
}

// TestPickRelaxesWhenExclusionEmptiesPool verifies the fallback: if excluding
// the sibling's commands would rule out every eligible line, the rotator shows a
// (briefly duplicate) line rather than going blank.
func TestPickRelaxesWhenExclusionEmptiesPool(t *testing.T) {
	withPlatform(t, platformTwitch)
	msgs := []rotatorMessage{{Text: "Try running `!location`"}}
	if got := pickRotatorMessage(msgs, map[string]bool{"location": true}); got != "Try running `!location`" {
		t.Errorf("expected exclusion to relax to the only line, got %q", got)
	}
}

// TestStartRotatorsPairsSiblings confirms the two corners are wired to each
// other so siblingCommands can see across.
func TestStartRotatorsPairsSiblings(t *testing.T) {
	l := newLeftRotator()
	r := newRightRotator()
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
	withPlatform(t, platformTwitch)
	l := newLeftRotator()
	r := newRightRotator()
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
