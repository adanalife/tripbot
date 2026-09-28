// Package findEvents holds the wire format for the operator find command:
// tripbot.<env>.find.run.<platform>, a NATS request/reply that runs !find's
// search-and-jump on one platform's stream without a word in chat and replies
// with the verdict. tripbot owns the search and the playhead, so it's the
// responder; the standalone tripbot-console is the requester.
//
// Like pkg/obs-events it is stdlib-only and side-effect-free: no init(), no
// pkg/config import, env is always a parameter — so it links safely into any
// binary.
package findEvents

import (
	"fmt"
	"time"
)

// RunSubject builds tripbot.<env>.find.run.<platform>. Per-platform because
// each platform's tripbot drives its own playout; only that instance answers.
func RunSubject(env, platform string) string {
	return fmt.Sprintf("tripbot.%s.find.run.%s", env, platform)
}

// Envelope is embedded in every find request. EmittedAt is an RFC3339Nano UTC
// timestamp, useful for latency/debugging.
type Envelope struct {
	EmittedAt string `json:"emitted_at"`
}

// NewEnvelope returns an Envelope stamped with the current UTC time.
func NewEnvelope() Envelope {
	return Envelope{EmittedAt: time.Now().UTC().Format(time.RFC3339Nano)}
}

// Run is the request: find query and jump the stream there.
type Run struct {
	Envelope
	Query string `json:"query"`
}

// Result is the reply. OK is true only when the stream jumped; Detail is a
// sentence worth reading back to the operator either way.
type Result struct {
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
}
