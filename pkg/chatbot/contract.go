package chatbot

import (
	"bytes"
	"encoding/json"
	"slices"
	"sort"
)

// The chat command registry, projected as data for consumers outside this
// binary. `pkg/contract`'s generator marshals it to chat-commands.json, which
// the admin console syncs the way it syncs contract.json / eventbus.json /
// commands.json.
//
// The projection is generated rather than hand-declared — unlike the wire
// schemas in pkg/contract, which describe a handful of stable fields, this is
// 45 rows of data that would rot the moment someone adds a command and forgets
// the second copy. pkg/contract still never imports this package — a shared
// package must not pull in the chatbot — so the generator is a main and no
// binary inherits the import.

// CommandSpec is one chat command as a consumer outside this binary sees it:
// how to invoke it, who may, and where it answers. Handlers and help copy are
// not part of it — a consumer renders and routes, it doesn't execute.
type CommandSpec struct {
	Trigger string   `json:"trigger"`
	Aliases []string `json:"aliases,omitempty"`
	// RequiresFollow / RequiresSubscriber are the access gate, as declared.
	// Whether a given viewer passes is a runtime question this can't answer,
	// and RequiresSubscriber is ignored outright on a platform with no
	// subscriber signal — see (*Command).checkAccess.
	RequiresFollow     bool `json:"requires_follow,omitempty"`
	RequiresSubscriber bool `json:"requires_subscriber,omitempty"`
	// RequiresAdmin marks a command only the broadcaster may run, so a
	// consumer can render !makebot differently from !weather.
	RequiresAdmin bool `json:"requires_admin,omitempty"`
	// Platforms is where this command actually dispatches, already resolved
	// from both gating rules (a command's own Platforms list, and the
	// platform's command scope). It is the resolved answer rather than the two
	// inputs so that a consumer filters instead of reimplementing the gate —
	// the gate has one implementation, here, and a second bot diffing this
	// registry against its own is comparing conclusions.
	Platforms []string `json:"platforms"`
}

// ChatCommands returns every registered command as a CommandSpec, ordered by
// trigger so the generated JSON is stable across runs. Registry order is
// authoring order and would reshuffle the file on an unrelated edit.
func ChatCommands() []CommandSpec {
	// buildRegistry only binds handler method values, so a zero App is enough
	// to read the declarations — no config, no database, no platform.
	registry := (&App{}).buildRegistry()

	platforms := knownPlatforms()
	specs := make([]CommandSpec, 0, len(registry))
	for i := range registry {
		cmd := &registry[i]
		specs = append(specs, CommandSpec{
			Trigger:            cmd.Trigger,
			Aliases:            cmd.Aliases,
			RequiresFollow:     cmd.RequiresFollow,
			RequiresSubscriber: cmd.RequiresSubscriber,
			RequiresAdmin:      cmd.RequiresAdmin,
			Platforms:          enabledPlatforms(cmd, platforms),
		})
	}
	sort.Slice(specs, func(i, j int) bool { return specs[i].Trigger < specs[j].Trigger })
	return specs
}

// chatCommandsComment heads the generated file, so anyone who opens it knows
// where it came from and that editing it is pointless.
const chatCommandsComment = "Generated from pkg/chatbot's command registry via `go generate ./pkg/contract` — do not hand-edit. " +
	"Every chat command tripbot answers: its trigger, aliases, access gate, and the platforms it dispatches on (already resolved). " +
	"Synced into the admin console alongside contract.json / eventbus.json / commands.json."

// chatCommands is the file's top-level shape.
type chatCommands struct {
	Comment  string        `json:"_comment"`
	Commands []CommandSpec `json:"commands"`
}

// MarshalChatCommands renders chat-commands.json: 2-space-indented JSON with a
// trailing newline (the bytes the generator writes and the golden test compares
// against).
func MarshalChatCommands() ([]byte, error) {
	out, err := json.MarshalIndent(chatCommands{
		Comment:  chatCommandsComment,
		Commands: ChatCommands(),
	}, "", "  ")
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	buf.Write(out)
	buf.WriteByte('\n')
	return buf.Bytes(), nil
}

// knownPlatforms is every platform the command gate can answer for, sorted.
// It reads platformCommandScope's keys rather than restating the set, so a
// platform added there is covered here without a second edit.
func knownPlatforms() []string {
	out := make([]string, 0, len(platformCommandScope))
	for p := range platformCommandScope {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// enabledPlatforms resolves where cmd dispatches, by asking the real gate once
// per platform. Going through commandEnabled rather than reimplementing its two
// rules is what keeps the contract honest: a change to the gate shows up in the
// generated file, and the golden test then fails until it's committed.
func enabledPlatforms(cmd *Command, platforms []string) []string {
	out := make([]string, 0, len(platforms))
	for _, p := range platforms {
		if (&App{Platform: p}).commandEnabled(cmd) {
			out = append(out, p)
		}
	}
	return slices.Clip(out)
}
