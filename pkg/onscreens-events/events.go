// Package onscreensEvents holds the wire format for the onscreens command
// events published over NATS (subjects of the form
// tripbot.<env>.onscreens.<overlay>.<verb>).
//
// It is imported by both the publisher (cmd/tripbot, via
// pkg/onscreens-client) and the subscriber (cmd/onscreens-server). To stay
// safe as a shared package it is stdlib-only and side-effect-free: no
// init(), no pkg/config import, env is always a parameter rather than read
// from config here.
package onscreensEvents

import "time"

// Envelope is embedded in every onscreens command event. EmittedAt is an
// RFC3339Nano UTC timestamp, useful for latency/debugging. Snake_case JSON
// so a future protobuf schema maps 1-1.
type Envelope struct {
	EmittedAt string `json:"emitted_at"`
}

// NewEnvelope returns an Envelope stamped with the current UTC time.
func NewEnvelope() Envelope {
	return Envelope{EmittedAt: time.Now().UTC().Format(time.RFC3339Nano)}
}

// MiddleShow is the payload for the middle.show subject.
type MiddleShow struct {
	Envelope
	Msg string `json:"msg"`
}

// MiddleState is the last-value state onscreens-server publishes about the
// middle-text overlay so the text (and its shown/hidden status) survives a
// server restart. Unlike MiddleShow — a *command* from tripbot — this is
// *state* the server emits about itself, read back from the
// MaxMsgsPerSubject=1 stream on startup. Mirrors the vlc lastplayed cache.
type MiddleState struct {
	Envelope
	Msg     string `json:"msg"`
	Showing bool   `json:"showing"`
}

// LeaderboardShow is the payload for the leaderboard.show subject. The
// server renders Rows into the on-screen HTML, so the wire carries
// structured data rather than a pre-rendered blob.
type LeaderboardShow struct {
	Envelope
	Title string     `json:"title"`
	Rows  [][]string `json:"rows"`
}

// TimewarpShow is the payload for the timewarp.show subject. Username is the
// chatter who triggered the warp (via !timewarp or a correct !guess); the
// overlay surfaces it as a credit line under the TIMEWARP wordmark. Empty
// when no user is attributable — the overlay then shows no credit. (The
// server still supplies the warp's duration; only the credit travels here.)
type TimewarpShow struct {
	Envelope
	Username string `json:"username"`
}

// Command is the payload for events that carry no data beyond the
// envelope: every hide, plus gps.show (the server supplies its content and
// duration). A single type rather than a swarm of identical empty structs —
// the subject distinguishes them, and any event that grows a field
// graduates to its own named type then (as timewarp.show did).
type Command struct {
	Envelope
}
