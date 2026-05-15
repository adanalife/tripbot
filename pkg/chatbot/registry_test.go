package chatbot

import (
	"strings"
	"testing"
)

func TestCommandsHaveNonEmptyTrigger(t *testing.T) {
	for _, cmd := range commands {
		if cmd.Trigger == "" {
			t.Errorf("command has empty Trigger: %+v", cmd)
		}
	}
}

func TestNoDuplicateTriggersOrAliases(t *testing.T) {
	seen := map[string]string{} // trigger/alias → owning command's Trigger
	for _, cmd := range commands {
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
	for _, cmd := range commands {
		all := append([]string{cmd.Trigger}, cmd.Aliases...)
		for _, token := range all {
			if strings.Contains(token, " ") {
				if _, ok := multiWordLookup[token]; !ok {
					t.Errorf("multi-word trigger %q missing from multiWordLookup", token)
				}
			} else {
				if _, ok := singleWordLookup[token]; !ok {
					t.Errorf("single-word trigger %q missing from singleWordLookup", token)
				}
			}
		}
	}
}

func TestLookupMapsPointToCorrectCommand(t *testing.T) {
	for i := range commands {
		cmd := &commands[i]
		all := append([]string{cmd.Trigger}, cmd.Aliases...)
		for _, token := range all {
			var got *Command
			if strings.Contains(token, " ") {
				got = multiWordLookup[token]
			} else {
				got = singleWordLookup[token]
			}
			if got != cmd {
				t.Errorf("trigger %q maps to %q, want %q", token, got.Trigger, cmd.Trigger)
			}
		}
	}
}
