// Package vlcEvents holds the wire format for the vlc-server command events
// published over NATS (subjects of the form tripbot.<env>.vlc.<verb>).
//
// It is imported by both the publisher (cmd/tripbot, via pkg/vlc-client) and
// the subscriber (cmd/vlc-server). To stay safe as a shared package it is
// stdlib-only and side-effect-free: no init(), no pkg/config import, env is
// always a parameter rather than read from config here.
//
// Scope: the fire-and-forget playback commands only (random / file / skip /
// back). The currently-playing read stays on HTTP — a request/response read
// doesn't fit fire-and-forget publish, and the current-video state already
// flows over NATS as video.changed (pkg/eventbus).
package vlcEvents

import "time"

// Envelope is embedded in every vlc command event. EmittedAt is an
// RFC3339Nano UTC timestamp, useful for latency/debugging. Snake_case JSON so
// a future protobuf schema maps 1-1. (Parallel to onscreensEvents.Envelope —
// per-domain wire packages keep their own copy.)
type Envelope struct {
	EmittedAt string `json:"emitted_at"`
}

// NewEnvelope returns an Envelope stamped with the current UTC time.
func NewEnvelope() Envelope {
	return Envelope{EmittedAt: time.Now().UTC().Format(time.RFC3339Nano)}
}

// Skip is the payload for the skip subject. N is the number of videos to
// advance (the client's Skip(n)).
type Skip struct {
	Envelope
	N int `json:"n"`
}

// Back is the payload for the back subject. N is the number of videos to
// rewind (the client's Back(n)).
type Back struct {
	Envelope
	N int `json:"n"`
}

// PlayFile is the payload for the play.file subject. File is the playlist
// filename to play (the client's PlayFileInPlaylist(file)).
type PlayFile struct {
	Envelope
	File string `json:"file"`
}

// Command is the payload for events that carry no data beyond the envelope:
// play.random (the server picks the file). A single type rather than an empty
// struct per subject — the subject distinguishes them, and any event that
// grows a field graduates to its own named type then.
type Command struct {
	Envelope
}

// LastPlayed is the payload for the lastplayed subject — the playlist
// basename vlc-server most recently started playing. Published by vlc-server
// itself on every successful play and read back on startup so a restarted
// instance resumes the clip it was on. Just the basename: the playlist is
// re-derived from disk on boot, so anything richer (state, GPS) would go
// stale; tripbot's video.changed remains the enriched observation event.
type LastPlayed struct {
	Envelope
	File string `json:"file"`
}
