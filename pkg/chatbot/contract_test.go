package chatbot

import (
	"slices"
	"testing"
)

// The byte-for-byte guard against the committed chat-commands.json lives in
// pkg/contract, next to the file and its three siblings. These cover the
// registry invariants behind it, which need this package's internals.

// The registry is the source consumers route on, so a duplicate trigger or a
// trigger colliding with another command's alias means one of the two is
// unreachable — indexCommands would silently let the later one win.
func TestChatCommandsHaveNoCollidingNames(t *testing.T) {
	seen := map[string]string{} // name -> the trigger that claimed it
	for _, spec := range ChatCommands() {
		for _, name := range append([]string{spec.Trigger}, spec.Aliases...) {
			if owner, dup := seen[name]; dup {
				t.Errorf("%q is claimed by both %s and %s; only one will dispatch", name, owner, spec.Trigger)
				continue
			}
			seen[name] = spec.Trigger
		}
	}
}

// Every command must dispatch somewhere. A command enabled on no platform is
// dead code that still looks registered — the shape a mistake in the gating
// rules takes.
func TestChatCommandsAllDispatchSomewhere(t *testing.T) {
	for _, spec := range ChatCommands() {
		if len(spec.Platforms) == 0 {
			t.Errorf("%s is enabled on no platform", spec.Trigger)
		}
	}
}

// The resolved platforms must agree with the gate the bot actually runs, per
// platform — the whole value of shipping resolved lists is that a consumer can
// trust them without reimplementing commandEnabled.
func TestChatCommandsPlatformsMatchTheGate(t *testing.T) {
	registry := (&App{}).buildRegistry()
	byTrigger := make(map[string]*Command, len(registry))
	for i := range registry {
		byTrigger[registry[i].Trigger] = &registry[i]
	}

	for _, spec := range ChatCommands() {
		cmd, ok := byTrigger[spec.Trigger]
		if !ok {
			t.Errorf("%s is in the contract but not the registry", spec.Trigger)
			continue
		}
		for _, platform := range knownPlatforms() {
			want := (&App{Platform: platform}).commandEnabled(cmd)
			if got := slices.Contains(spec.Platforms, platform); got != want {
				t.Errorf("%s on %s: contract says %v, commandEnabled says %v",
					spec.Trigger, platform, got, want)
			}
		}
	}
}
