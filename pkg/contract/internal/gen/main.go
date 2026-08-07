// Command gen marshals the canonical pkg/contract constants to
// pkg/contract/contract.json. It is invoked by the //go:generate directive in
// pkg/contract/generate.go; run `go generate ./pkg/contract` after editing the
// constants.
package main

import (
	"log"
	"os"
	"path/filepath"
	"runtime"

	"github.com/adanalife/tripbot/pkg/chatbot"
	"github.com/adanalife/tripbot/pkg/contract"
)

func main() {
	data, err := contract.Current().Marshal()
	if err != nil {
		log.Fatalf("marshal contract: %v", err)
	}

	// Resolve the output path relative to this source file so the generator
	// writes to pkg/contract/contract.json regardless of the working directory
	// `go generate` runs from.
	_, self, _, ok := runtime.Caller(0)
	if !ok {
		log.Fatal("could not determine generator source path")
	}
	// self = pkg/contract/internal/gen/main.go → pkg/contract/contract.json
	out := filepath.Join(filepath.Dir(self), "..", "..", "contract.json")

	if err := os.WriteFile(out, data, 0o644); err != nil {
		log.Fatalf("write %s: %v", out, err)
	}
	log.Printf("wrote %s", out)

	// The eventbus registry (NATS subjects + envelope JSON Schemas) is emitted
	// alongside the service/port contract — a sibling eventbus.json the console
	// syncs to discover subjects + payload shapes.
	ebData, err := contract.MarshalEventbus()
	if err != nil {
		log.Fatalf("marshal eventbus: %v", err)
	}
	ebOut := filepath.Join(filepath.Dir(self), "..", "..", "eventbus.json")
	if err := os.WriteFile(ebOut, ebData, 0o644); err != nil {
		log.Fatalf("write %s: %v", ebOut, err)
	}
	log.Printf("wrote %s", ebOut)

	// The onscreens command registry (subjects onscreens-server subscribes to +
	// their envelope schemas) is emitted alongside — a sibling commands.json the
	// console syncs to publish overlay commands without hand-building subjects.
	cmdData, err := contract.MarshalCommands()
	if err != nil {
		log.Fatalf("marshal commands: %v", err)
	}
	cmdOut := filepath.Join(filepath.Dir(self), "..", "..", "commands.json")
	if err := os.WriteFile(cmdOut, cmdData, 0o644); err != nil {
		log.Fatalf("write %s: %v", cmdOut, err)
	}
	log.Printf("wrote %s", cmdOut)

	// The chat command registry (every !command tripbot answers, with its
	// access gate and resolved platforms) is emitted alongside — a sibling
	// chat-commands.json the console syncs to render the live command list.
	//
	// Unlike the three above, its source is pkg/chatbot rather than a
	// declaration in pkg/contract: it's registry data, not a wire schema, and a
	// hand-kept second copy of 45 commands would drift the first time someone
	// adds one. The import is safe here and only here — this generator is a
	// main nothing links, so pkg/contract stays free of it and no binary
	// inherits the chatbot's dependency tree.
	chatData, err := chatbot.MarshalChatCommands()
	if err != nil {
		log.Fatalf("marshal chat commands: %v", err)
	}
	chatOut := filepath.Join(filepath.Dir(self), "..", "..", "chat-commands.json")
	if err := os.WriteFile(chatOut, chatData, 0o644); err != nil {
		log.Fatalf("write %s: %v", chatOut, err)
	}
	log.Printf("wrote %s", chatOut)
}
