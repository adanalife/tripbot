// Package obsEvents holds the wire format for OBS *command* events published
// over NATS — operator-initiated imperatives that drive tripbot's OBS WebSocket
// connection. The first is obs.refresh (tripbot.<env>.obs.refresh.<platform>):
// "hard-reload every browser source on this platform's OBS". tripbot owns the
// OBS WebSocket, so it's the subscriber; the standalone tripbot-console is the
// publisher (a settings-page button), the same command split as pkg/chat-events.
//
// Like pkg/chat-events, pkg/playout-events and pkg/onscreens-events it is
// stdlib-only and side-effect-free: no init(), no pkg/config import, env is
// always a parameter rather than read from config — so it links safely into any
// binary.
package obsEvents

import "time"

// Envelope is embedded in every obs command event. EmittedAt is an RFC3339Nano
// UTC timestamp, useful for latency/debugging. Snake_case JSON so a future
// protobuf schema maps 1-1.
type Envelope struct {
	EmittedAt string `json:"emitted_at"`
}

// NewEnvelope returns an Envelope stamped with the current UTC time.
func NewEnvelope() Envelope {
	return Envelope{EmittedAt: time.Now().UTC().Format(time.RFC3339Nano)}
}

// Refresh is the payload for the obs.refresh subject: hard-reload every OBS
// browser source. It carries no fields beyond the envelope — the subject (with
// its platform leaf) is the whole command. Fire-and-forget; no reply.
type Refresh struct {
	Envelope
}
