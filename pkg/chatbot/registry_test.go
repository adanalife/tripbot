package chatbot

import (
	"strings"
	"testing"
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
	for _, token := range []string{"!weather", "!meteo", "!skip", "!timewarp", "!warp", "!youtube"} {
		if cmd, _ := yt.findCommand(token); cmd == nil {
			t.Errorf("expected %q to be available on YouTube, got nil", token)
		}
	}

	// excluded: identity/miles, Twitch-only, admin, and the deferred
	// now-playing commands do not resolve
	for _, token := range []string{"!miles", "!km", "!leaderboard", "!guess", "!state", "!location", "!followage", "!middle", "!shutdown", "!makebot", "hello", "!song", "!music", "!somafm"} {
		if cmd, _ := yt.findCommand(token); cmd != nil {
			t.Errorf("expected %q to be unavailable on YouTube, got %q", token, cmd.Trigger)
		}
	}
}
