// Package leaderboardEvents holds the wire format for leaderboard *command*
// events published over NATS — "put this board on screen now", operator-
// initiated. tripbot owns the scoreboard tables and the overlay titles, so it
// is the subscriber; the standalone tripbot-console is the publisher (an
// overlay-panel button), the same command split as pkg/obs-events.
//
// The onscreens registry already carries an onscreens.leaderboard.show subject,
// but that one takes the rendered rows — only tripbot can produce them, because
// the boards come out of its database with its bot/opt-out filtering applied.
// This subject carries the board's *name* instead, and tripbot answers it by
// publishing the onscreens command.
//
// Like pkg/chat-events, pkg/obs-events, pkg/playout-events and
// pkg/onscreens-events it is stdlib-only and side-effect-free: no init(), no
// pkg/config import, env is always a parameter rather than read from config —
// so it links safely into any binary.
package leaderboardEvents

import "time"

// Envelope is embedded in every leaderboard command event. EmittedAt is an
// RFC3339Nano UTC timestamp, useful for latency/debugging. Snake_case JSON so a
// future protobuf schema maps 1-1.
type Envelope struct {
	EmittedAt string `json:"emitted_at"`
}

// NewEnvelope returns an Envelope stamped with the current UTC time.
func NewEnvelope() Envelope {
	return Envelope{EmittedAt: time.Now().UTC().Format(time.RFC3339Nano)}
}

// Board names, the wire vocabulary for Show.Board. They name the same boards
// the onscreen rotation picks between, so a button and a rotation tick put the
// identical overlay up.
const (
	BoardMiles         = "miles"
	BoardTotalMiles    = "total_miles"
	BoardGuesses       = "guesses"
	BoardGuessrDaily   = "guessr_daily"
	BoardGuessrMonthly = "guessr_monthly"
)

// Boards is every value Show.Board accepts, in the order a picker should offer
// them: the two boards viewers climb all month first, then the rarities.
var Boards = []string{
	BoardMiles,
	BoardGuesses,
	BoardTotalMiles,
	BoardGuessrDaily,
	BoardGuessrMonthly,
}

// Show is the payload for the leaderboard.show subject: put Board on screen.
// Fire-and-forget; no reply. A board that is empty right now (the guess board
// early in the month, a guessr board while the game is unreachable) shows
// nothing rather than an empty frame — the same rule the rotation tick follows.
type Show struct {
	Envelope
	Board string `json:"board"`
}
