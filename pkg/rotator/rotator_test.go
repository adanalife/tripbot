package rotator

import (
	"strings"
	"testing"
)

func TestMessageAppliesTo(t *testing.T) {
	all := Message{Text: "x"}
	if !all.AppliesTo(PlatformYouTube) || !all.AppliesTo(PlatformTwitch) {
		t.Error("empty Platforms should apply to all platforms")
	}
	tw := Message{Text: "x", Platforms: []string{PlatformTwitch}}
	if tw.AppliesTo(PlatformYouTube) {
		t.Error("twitch-only message should not apply to YouTube")
	}
	if !tw.AppliesTo(PlatformTwitch) {
		t.Error("twitch-only message should apply to Twitch")
	}
}

// TestDefaultsOmitTwitchOnlyOnYouTube guards the headline behavior: a YouTube
// overlay must never surface the !miles / !guess lines, which would advertise
// commands disabled on that platform.
func TestDefaultsOmitTwitchOnlyOnYouTube(t *testing.T) {
	pool := DefaultConfig().Left.Messages
	for i := 0; i < 2000; i++ {
		msg := Pick(PlatformYouTube, pool, nil, nil)
		if strings.Contains(msg, "!miles") || strings.Contains(msg, "!guess") {
			t.Fatalf("YouTube left rotator surfaced a Twitch-only line: %q", msg)
		}
	}
}

// TestDefaultsSurfaceTwitchOnlyOnTwitch confirms the Twitch-only lines are still
// reachable on Twitch (the filter doesn't drop them everywhere).
func TestDefaultsSurfaceTwitchOnlyOnTwitch(t *testing.T) {
	pool := DefaultConfig().Left.Messages
	var sawMiles, sawGuess bool
	for i := 0; i < 5000 && !(sawMiles && sawGuess); i++ {
		switch Pick(PlatformTwitch, pool, nil, nil) {
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

func TestPickEmptyWhenNoneApply(t *testing.T) {
	twitchOnly := []Message{
		{Text: "a", Platforms: []string{PlatformTwitch}},
		{Text: "b", Platforms: []string{PlatformTwitch}},
	}
	if got := Pick(PlatformYouTube, twitchOnly, nil, nil); got != "" {
		t.Errorf("expected empty string when no message applies, got %q", got)
	}
}

func TestPickEmptyPool(t *testing.T) {
	if got := Pick(PlatformTwitch, nil, nil, nil); got != "" {
		t.Errorf("expected empty string for an empty pool, got %q", got)
	}
}

// TestPickRespectsWeight checks the weighted draw is biased: a Weight:9 entry
// should dominate a Weight:1 entry over many samples.
func TestPickRespectsWeight(t *testing.T) {
	msgs := []Message{
		{Text: "rare"},              // weight 1
		{Text: "common", Weight: 9}, // weight 9
	}
	var common int
	const n = 10000
	for i := 0; i < n; i++ {
		if Pick(PlatformTwitch, msgs, nil, nil) == "common" {
			common++
		}
	}
	// Expect ~90%; allow generous slack to stay non-flaky.
	if common < n*3/4 {
		t.Errorf("weighted draw not biased: common=%d/%d", common, n)
	}
}

func TestWeightedTreatsUnsetAsOne(t *testing.T) {
	if got := (Message{}).Weighted(); got != 1 {
		t.Errorf("unset Weight = %d, want 1", got)
	}
	if got := (Message{Weight: -3}).Weighted(); got != 1 {
		t.Errorf("negative Weight = %d, want 1", got)
	}
}

func TestCommandsIn(t *testing.T) {
	cmds := CommandsIn("Where are we? (`!location`) and try `!timewarp`")
	if !cmds["location"] || !cmds["timewarp"] {
		t.Errorf("expected location+timewarp, got %v", cmds)
	}
	// A bare "!" as punctuation is not a command token.
	if got := CommandsIn(DefaultRareMessage); got != nil {
		t.Errorf("expected no commands in punctuation-only text, got %v", got)
	}
	if got := CommandsIn("twitch.tv/ADanaLife_"); got != nil {
		t.Errorf("expected no commands, got %v", got)
	}
}

// TestPickExcludesSiblingCommand is the headline of the dedup feature: when the
// sibling corner is already showing !location, this corner must never pick a
// line advertising !location — the two corners shouldn't echo the same command.
func TestPickExcludesSiblingCommand(t *testing.T) {
	pool := DefaultConfig().Right.Messages
	exclude := map[string]bool{"location": true}
	for i := 0; i < 4000; i++ {
		if got := Pick(PlatformTwitch, pool, exclude, nil); got == "Try running `!location`" {
			t.Fatalf("right rotator surfaced !location while sibling shows it: %q", got)
		}
	}
}

// TestPickRelaxesWhenExclusionEmptiesPool verifies the fallback: if excluding the
// sibling's commands would rule out every eligible line, the rotator shows a
// (briefly duplicate) line rather than going blank.
func TestPickRelaxesWhenExclusionEmptiesPool(t *testing.T) {
	msgs := []Message{{Text: "Try running `!location`"}}
	if got := Pick(PlatformTwitch, msgs, map[string]bool{"location": true}, nil); got != "Try running `!location`" {
		t.Errorf("expected exclusion to relax to the only line, got %q", got)
	}
}

// TestDefaultConfigForFiltersAndClearsScoping covers what the console prefills a
// fresh platform editor with: only the lines that platform would ever render,
// with the now-moot Platforms scoping dropped (stored copy is per-platform).
func TestDefaultConfigForFiltersAndClearsScoping(t *testing.T) {
	yt := DefaultConfigFor(PlatformYouTube)
	for _, m := range yt.Left.Messages {
		if len(m.Platforms) != 0 {
			t.Errorf("per-platform prefill kept Platforms scoping on %q: %v", m.Text, m.Platforms)
		}
		if strings.Contains(m.Text, "!miles") || strings.Contains(m.Text, "!guess") {
			t.Errorf("YouTube prefill included a Twitch-only line: %q", m.Text)
		}
	}
	// TikTok's gift lines are scoped to TikTok, so they belong in its prefill and
	// nowhere else.
	var tiktokGift, youtubeGift bool
	for _, m := range DefaultConfigFor(PlatformTikTok).Left.PromoMessages {
		if strings.Contains(m.Text, "gift") || strings.Contains(m.Text, "Gift") {
			tiktokGift = true
		}
	}
	for _, m := range yt.Left.PromoMessages {
		if strings.Contains(m.Text, "gift") || strings.Contains(m.Text, "Gift") {
			youtubeGift = true
		}
	}
	if !tiktokGift {
		t.Error("TikTok prefill should include the gift lines")
	}
	if youtubeGift {
		t.Error("YouTube prefill should not include TikTok's gift lines")
	}
}

// TestDefaultConfigIsolatesCallers guards the deep copy: the console round-trips
// this struct and mutates it, which must not reach back into the package defaults.
func TestDefaultConfigIsolatesCallers(t *testing.T) {
	first := DefaultConfig()
	original := first.Left.Messages[0].Text
	first.Left.Messages[0].Text = "scribbled"
	if got := DefaultConfig().Left.Messages[0].Text; got != original {
		t.Errorf("mutating a returned config changed the defaults: %q", got)
	}
}
