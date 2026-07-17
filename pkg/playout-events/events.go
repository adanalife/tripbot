// Package playoutEvents holds the wire format for the playout command events
// published over NATS (subjects of the form tripbot.<env>.vlc.<verb> — the
// "vlc" segment is the legacy wire name playout serves; it renames with the
// coordinated contract change).
//
// It is imported by the publisher (cmd/tripbot, via pkg/playout-client); the
// subscriber is the playout server (adanalife/playout repo). To stay safe as
// a shared package it is
// stdlib-only and side-effect-free: no init(), no pkg/config import, env is
// always a parameter rather than read from config here.
//
// Scope: the fire-and-forget playback commands only (random / file / skip /
// back). The currently-playing read stays on HTTP — a request/response read
// doesn't fit fire-and-forget publish, and the current-video state already
// flows over NATS as video.changed (pkg/eventbus).
package playoutEvents

import "time"

// Envelope is embedded in every playout command event. EmittedAt is an
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

// Seek is the payload for the seek subject — move the playhead by DeltaMs
// milliseconds relative to the current playback position, crossing clip
// boundaries as needed; negative rewinds. The server walks real clip
// durations, so a duration-based !skip/!back lands where that much footage
// actually is (the client's Seek(delta)).
type Seek struct {
	Envelope
	DeltaMs int64 `json:"delta_ms"`
}

// PlayFile is the payload for the play.file subject. File is the playlist
// filename to play (the client's PlayFileInPlaylist(file)).
type PlayFile struct {
	Envelope
	File string `json:"file"`
}

// PlayFileAt is the payload for the play.at subject — play File and then seek
// to PositionMs within it (the client's PlayFileAtTimestamp(file, ts)). Unlike
// play.file (which always starts at the top), this carries a seek target so
// !find can jump straight to the matching moment. PositionMs 0 / omitted means
// start-of-clip, identical to play.file.
type PlayFileAt struct {
	Envelope
	File string `json:"file"`
	// PositionMs is the seek target within File in milliseconds.
	PositionMs int64 `json:"position_ms,omitempty"`
}

// Command is the payload for events that carry no data beyond the envelope:
// play.random (the server picks the file). A single type rather than an empty
// struct per subject — the subject distinguishes them, and any event that
// grows a field graduates to its own named type then.
type Command struct {
	Envelope
}

// LastPlayed is the payload for the lastplayed subject — the playlist
// basename playout most recently started playing, plus how far in it was.
// Published by playout itself (at clip start and on a periodic position
// ticker) and read back on startup so a restarted instance resumes the clip —
// and the spot — it was on. Just the basename: the playlist is re-derived
// from disk on boot, so anything richer (state, GPS) would go stale;
// tripbot's video.changed remains the enriched observation event.
type LastPlayed struct {
	Envelope
	File string `json:"file"`
	// PositionMs is the playback position within File in milliseconds.
	// 0 / omitted means start-of-clip — which is also what messages published
	// before this field existed decode to.
	PositionMs int64 `json:"position_ms,omitempty"`
}
