package chatbot

import (
	"context"
	"log/slog"

	"github.com/adanalife/tripbot/pkg/video"
)

// spot is where the stream is: the clip on screen and the moment showing at
// this instant. atPlayhead says the coordinate came from the clip's per-moment
// track rather than its single clip-level fix — which is what makes it precise
// enough to be worth naming in its own right instead of reading the state
// already recorded for the clip.
type spot struct {
	vid video.Video
	at  video.Moment

	atPlayhead bool
}

// currentSpot answers "where are we" for the commands that need a coordinate.
// It reads the per-moment track when the clip has a trusted one and falls back
// to the clip's single fix otherwise.
//
// A flagged clip has no usable GPS at all, so it walks forward to the next clip
// that does; when even that fails it tells chat and reports ok=false, leaving
// the caller nothing to do but return.
func (a *App) currentSpot(ctx context.Context) (spot, bool) {
	vid, at, atPlayhead := a.Video.PlayheadLocation(ctx)

	if vid.Flagged {
		a.Chat.Say("I couldn't figure out current GPS coords, using next closest...")
		//TODO: write something like vid.FindClosest() that
		// chooses whether or not to use Next() vs Prev()
		next, err := vid.NextUnflagged(ctx)
		if err != nil {
			slog.ErrorContext(ctx, "error finding next unflagged video", "err", err)
			a.Chat.Say("I couldn't figure out current GPS coords, sorry!")
			return spot{}, false
		}
		// The playhead is inside the flagged clip, so its offset says nothing
		// about where we are in this substitute one.
		vid, at, atPlayhead = next, video.Moment{}, false
	}

	if !atPlayhead {
		at.Lat, at.Lng, _ = vid.Location()
	}
	return spot{vid: vid, at: at, atPlayhead: atPlayhead}, true
}

// place is the human-readable location of the spot — "Bishop, California",
// "near Mammoth, Wyoming", or "Somewhere in Wyoming".
//
// The pipeline resolves this offline at ingest and stores it on the row, so the
// common path costs no API call at all. The live geocoder is the fallback for a
// moment the geocode pass hasn't reached, and for clips answering from their
// clip-level fix.
func (a *App) place(ctx context.Context, s spot) string {
	if p := s.at.Place(); p != "" {
		return p
	}
	address, err := a.Geocoder.City(s.at.Lat, s.at.Lng)
	if err != nil {
		slog.ErrorContext(ctx, "geocoding error", "err", err)
	}
	return address
}

// state is the US state the spot is in.
//
// Same story as place: the pipeline resolved it from the same Census boundaries
// the footage is contemporary with, so a clip that crosses a state line answers
// for the half on screen without asking anyone. Falling back needs care — a
// per-moment coordinate is worth reverse-geocoding, but a clip-level one is
// already what videos.state was geocoded from, so asking again would spend a
// Maps call to get the same answer back.
//
// Never returns empty when the clip has a state: an empty answer makes guessCmd
// refuse every guess, so a Maps outage would take !guess down with it. Both
// !guess and !state route through here, so grading and reveal can't disagree.
func (a *App) state(ctx context.Context, s spot) string {
	if s.at.State != "" {
		return s.at.State
	}
	if !s.atPlayhead {
		return s.vid.State
	}
	state, err := a.Geocoder.State(s.at.Lat, s.at.Lng)
	if err != nil {
		slog.WarnContext(ctx, "playhead state lookup failed, using the clip's",
			"err", err, "slug", s.vid.Slug)
		return s.vid.State
	}
	if state == "" {
		return s.vid.State
	}
	return state
}
