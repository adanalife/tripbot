package main

// playerVideoSource is the cmd-wired users.VideoSource, reading the
// process-wide player so login/logout events record the footage that was on
// screen. It reads the Player's cached state rather than asking playout, since
// a session tick must not block on an HTTP round-trip.
//
// It holds *Tripbot rather than the player directly so it can be wired against
// a partially-built t, the same way gatewayChatterSource is.
type playerVideoSource struct{ t *Tripbot }

func (s playerVideoSource) CurrentVideoID() int {
	if s.t.player == nil {
		return 0
	}
	return s.t.player.Current().ID
}

func (s playerVideoSource) CurrentProgressSec() float64 {
	if s.t.player == nil {
		return 0
	}
	return s.t.player.CurrentProgress().Seconds()
}
