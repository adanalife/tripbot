package chatbot

import (
	"context"
	"regexp"
	"strings"
	"testing"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
)

func TestCommandsHaveNonEmptyTrigger(t *testing.T) {
	for _, cmd := range builtTestApp.commands {
		if cmd.Trigger == "" {
			t.Errorf("command has empty Trigger: %+v", cmd)
		}
	}
}

func TestNoDuplicateTriggersOrAliases(t *testing.T) {
	seen := map[string]string{} // trigger/alias → owning command's Trigger
	for _, cmd := range builtTestApp.commands {
		all := append([]string{cmd.Trigger}, cmd.Aliases...)
		for _, token := range all {
			if owner, clash := seen[token]; clash {
				t.Errorf("duplicate trigger/alias %q: claimed by %q and %q", token, owner, cmd.Trigger)
			}
			seen[token] = cmd.Trigger
		}
	}
}

func TestLookupMapsContainAllTriggers(t *testing.T) {
	for _, cmd := range builtTestApp.commands {
		// builtTestApp is a Twitch App; commands scoped to other platforms via
		// Command.Platforms (like !carsound on YouTube) aren't indexed here.
		if !builtTestApp.commandEnabled(&cmd) {
			continue
		}
		all := append([]string{cmd.Trigger}, cmd.Aliases...)
		for _, token := range all {
			if strings.Contains(token, " ") {
				if _, ok := builtTestApp.multiWordLookup[token]; !ok {
					t.Errorf("multi-word trigger %q missing from multiWordLookup", token)
				}
			} else {
				if _, ok := builtTestApp.singleWordLookup[token]; !ok {
					t.Errorf("single-word trigger %q missing from singleWordLookup", token)
				}
			}
		}
	}
}

func TestLookupMapsPointToCorrectCommand(t *testing.T) {
	for i := range builtTestApp.commands {
		cmd := &builtTestApp.commands[i]
		// Skip commands scoped to other platforms via Command.Platforms — they
		// aren't in builtTestApp's (Twitch) lookup maps by design.
		if !builtTestApp.commandEnabled(cmd) {
			continue
		}
		all := append([]string{cmd.Trigger}, cmd.Aliases...)
		for _, token := range all {
			var got *Command
			if strings.Contains(token, " ") {
				got = builtTestApp.multiWordLookup[token]
			} else {
				got = builtTestApp.singleWordLookup[token]
			}
			if got != cmd {
				t.Errorf("trigger %q maps to %q, want %q", token, got.Trigger, cmd.Trigger)
			}
		}
	}
}

// TestYouTubeAllowlistTriggersExist guards against drift: every trigger in the
// v1 allowlist must be a real command, so a rename or removal can't
// silently leave a dangling allowlist entry. Indexed on a YouTube App because
// the allowlist is consulted on the YouTube platform.
func TestYouTubeAllowlistTriggersExist(t *testing.T) {
	yt := &App{Platform: platformYouTube}
	yt.indexCommands()
	for trigger := range v1Commands {
		if _, ok := yt.singleWordLookup[trigger]; !ok {
			t.Errorf("v1Commands trigger %q is not a real command in the registry", trigger)
		}
	}
}

// TestFacebookPlatformIndexesOnlyAllowlist verifies a Facebook App runs the
// same v1 cross-platform allowlist, and that platform-scoped commands stay on
// their platform (!carsound is YouTube-only).
func TestFacebookPlatformIndexesOnlyAllowlist(t *testing.T) {
	fb := &App{Platform: platformFacebook}
	fb.indexCommands()
	for _, token := range []string{"!skip", "!timewarp", "!location", "!facebook"} {
		if cmd, _ := fb.findCommand(token); cmd == nil {
			t.Errorf("expected %q to be available on Facebook, got nil", token)
		}
	}
	for _, token := range []string{"!miles", "!guess", "!shutdown", "!carsound"} {
		if cmd, _ := fb.findCommand(token); cmd != nil {
			t.Errorf("expected %q to be unavailable on Facebook, got %q", token, cmd.Trigger)
		}
	}
}

// TestInstagramPlatformIndexesOnlyAllowlist verifies an Instagram App runs the
// same v1 cross-platform allowlist, and that platform-scoped commands stay on
// their platform (!carsound is YouTube-only).
func TestInstagramPlatformIndexesOnlyAllowlist(t *testing.T) {
	ig := &App{Platform: platformInstagram}
	ig.indexCommands()
	for _, token := range []string{"!skip", "!timewarp", "!location", "!instagram"} {
		if cmd, _ := ig.findCommand(token); cmd == nil {
			t.Errorf("expected %q to be available on Instagram, got nil", token)
		}
	}
	for _, token := range []string{"!miles", "!guess", "!shutdown", "!carsound"} {
		if cmd, _ := ig.findCommand(token); cmd != nil {
			t.Errorf("expected %q to be unavailable on Instagram, got %q", token, cmd.Trigger)
		}
	}
}

// TestYouTubePlatformIndexesOnlyAllowlist verifies a YouTube App dispatches the
// v1 cross-platform allowlist (triggers + their aliases) plus its
// platform-scoped commands, and nothing else — identity/miles, the Twitch-only
// !followage, and admin commands must not resolve.
func TestYouTubePlatformIndexesOnlyAllowlist(t *testing.T) {
	yt := &App{Platform: platformYouTube}
	yt.indexCommands()

	// allowed: a trigger and one of its aliases both resolve. The first set are
	// cross-platform allowlist entries; !carsound/!carhum are YouTube-scoped via
	// Command.Platforms.
	for _, token := range []string{"!weather", "!meteo", "!skip", "!timewarp", "!warp", "!youtube", "!state", "!location", "!where", "!carsound", "!carhum"} {
		if cmd, _ := yt.findCommand(token); cmd == nil {
			t.Errorf("expected %q to be available on YouTube, got nil", token)
		}
	}

	// excluded: identity/miles, Twitch-only, admin, and the deferred
	// now-playing commands do not resolve
	for _, token := range []string{"!miles", "!km", "!leaderboard", "!guess", "!followage", "!middle", "!shutdown", "!makebot", "hello", "!song", "!music", "!somafm"} {
		if cmd, _ := yt.findCommand(token); cmd != nil {
			t.Errorf("expected %q to be unavailable on YouTube, got %q", token, cmd.Trigger)
		}
	}
}

// TestTikTokPlatformIndexesOnlyAllowlist verifies a TikTok App runs the same
// v1 cross-platform allowlist, and that platform-scoped commands stay on their
// platform (!carsound is YouTube-only).
func TestTikTokPlatformIndexesOnlyAllowlist(t *testing.T) {
	tk := &App{Platform: platformTikTok}
	tk.indexCommands()
	for _, token := range []string{"!skip", "!timewarp", "!location", "!tiktok"} {
		if cmd, _ := tk.findCommand(token); cmd == nil {
			t.Errorf("expected %q to be available on TikTok, got nil", token)
		}
	}
	for _, token := range []string{"!miles", "!guess", "!shutdown", "!carsound"} {
		if cmd, _ := tk.findCommand(token); cmd != nil {
			t.Errorf("expected %q to be unavailable on TikTok, got %q", token, cmd.Trigger)
		}
	}
}

// TestUndeclaredPlatformDefaultsToAllowlist is the capability-driven gate's
// safety property: a platform absent from platformCommandScope (e.g. a
// freshly-wired Kick instance) is restricted to the v1 allowlist rather than
// falling through to the full command surface. Without the conservative default
// a new STREAM_PLATFORM would silently inherit every command, including the
// identity/miles and admin commands its backend can't support yet.
func TestUndeclaredPlatformDefaultsToAllowlist(t *testing.T) {
	app := &App{Platform: "kick"} // deliberately not in platformCommandScope
	app.indexCommands()

	// allowlisted cross-platform commands resolve
	for _, token := range []string{"!weather", "!timewarp", "!state", "!location"} {
		if cmd, _ := app.findCommand(token); cmd == nil {
			t.Errorf("expected allowlisted %q to be available on an undeclared platform, got nil", token)
		}
	}
	// full-surface-only commands must not resolve on an undeclared platform
	for _, token := range []string{"!miles", "!leaderboard", "!guess", "!middle", "!shutdown"} {
		if cmd, _ := app.findCommand(token); cmd != nil {
			t.Errorf("undeclared platform should not run full-surface %q, got %q", token, cmd.Trigger)
		}
	}
}

// TestCommandsCmdFiltersByPlatform verifies the !commands reply advertises only
// commands dispatchable on the App's platform — Twitch lists the disabled-on-
// YouTube ones (!guess, !miles, !leaderboard); YouTube does not, but still
// includes the YouTube-enabled !state / !location.
func TestCommandsCmdFiltersByPlatform(t *testing.T) {
	twitch := &App{}
	twitch.indexCommands()
	twChat := &recordingChat{}
	twitch.Chat = twChat
	twitch.commandsCmd(context.Background(), nil, nil)
	twOut := twChat.Output()
	for _, want := range []string{"!guess", "!miles", "!leaderboard", "!state", "!location"} {
		if !strings.Contains(twOut, want) {
			t.Errorf("Twitch !commands missing %q: %q", want, twOut)
		}
	}

	yt := &App{Platform: platformYouTube}
	yt.indexCommands()
	ytChat := &recordingChat{}
	yt.Chat = ytChat
	yt.commandsCmd(context.Background(), nil, nil)
	ytOut := ytChat.Output()
	for _, absent := range []string{"!guess", "!miles", "!leaderboard", "!song"} {
		if strings.Contains(ytOut, absent) {
			t.Errorf("YouTube !commands should not advertise %q: %q", absent, ytOut)
		}
	}
	for _, want := range []string{"!state", "!location"} {
		if !strings.Contains(ytOut, want) {
			t.Errorf("YouTube !commands missing %q: %q", want, ytOut)
		}
	}
}

// TestEnabledHelpMessagesFiltersByPlatform verifies the rotating help lines drop
// any whose command isn't dispatchable on the platform — so a YouTube instance
// never advertises !miles / !guess / !leaderboard via !help or the Chatter cron.
func TestEnabledHelpMessagesFiltersByPlatform(t *testing.T) {
	twitch := &App{}
	twitch.indexCommands()
	if len(twitch.helpMessages) != len(c.HelpMessages) {
		t.Errorf("Twitch should keep all %d help messages, got %d", len(c.HelpMessages), len(twitch.helpMessages))
	}

	yt := &App{Platform: platformYouTube}
	yt.indexCommands()
	for _, msg := range yt.helpMessages {
		for _, banned := range []string{"!miles", "!guess", "!leaderboard"} {
			if strings.HasPrefix(msg, banned) {
				t.Errorf("YouTube help message advertises disabled %q: %q", banned, msg)
			}
		}
	}
	if len(yt.helpMessages) == 0 {
		t.Error("YouTube help messages unexpectedly empty")
	}
}

// TestBotlessHelpMessagesAdvertiseNoCommands verifies a bot-less instance
// (YouTube with inbound chat disabled) swaps its rotating help lines for the
// promo set and advertises no "!command" token — typing a command into an
// unread YouTube chat would look like a broken bot.
func TestBotlessHelpMessagesAdvertiseNoCommands(t *testing.T) {
	yt := &App{Platform: platformYouTube, botless: true}
	yt.indexCommands()

	if len(yt.helpMessages) != len(c.YouTubeBotlessHelpMessages) {
		t.Fatalf("bot-less help should be the promo set (%d lines), got %d",
			len(c.YouTubeBotlessHelpMessages), len(yt.helpMessages))
	}
	for _, msg := range yt.helpMessages {
		if commandToken.MatchString(msg) {
			t.Errorf("bot-less help line advertises a command: %q", msg)
		}
	}
}

// commandToken matches a chat-command token ("!" followed by a letter, e.g.
// !location) without matching a bare "!" used as punctuation.
var commandToken = regexp.MustCompile(`![a-zA-Z]`)
