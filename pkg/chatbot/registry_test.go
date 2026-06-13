package chatbot

import (
	"context"
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
// YouTube allowlist must be a real command in the full registry, so a rename or
// removal can't silently leave a dangling allowlist entry.
func TestYouTubeAllowlistTriggersExist(t *testing.T) {
	full := &App{} // empty platform → full registry
	full.indexCommands()
	for trigger := range youtubeCommands {
		if _, ok := full.singleWordLookup[trigger]; !ok {
			t.Errorf("youtubeCommands trigger %q is not a real command in the registry", trigger)
		}
	}
}

// TestYouTubePlatformIndexesOnlyAllowlist verifies a YouTube App dispatches the
// v1 allowlist (triggers + their aliases) and nothing else — identity/miles,
// the Twitch-only !followage, and admin commands must not resolve.
func TestYouTubePlatformIndexesOnlyAllowlist(t *testing.T) {
	yt := &App{Platform: platformYouTube}
	yt.indexCommands()

	// allowed: a trigger and one of its aliases both resolve
	for _, token := range []string{"!weather", "!meteo", "!skip", "!timewarp", "!warp", "!youtube", "!state", "!location", "!where"} {
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
