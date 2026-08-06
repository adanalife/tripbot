package chatbot

import (
	"context"
	"log/slog"

	"github.com/adanalife/tripbot/pkg/video"
)

// spot is where the stream is: the clip on screen and the coordinate showing
// at this moment. atPlayhead says that coordinate came from the clip's
// per-moment track rather than its single clip-level fix — which is what makes
// it precise enough to be worth reverse-geocoding instead of reading the state
// already recorded for the clip.
type spot struct {
	vid      video.Video
	lat, lng float64

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
	vid, lat, lng, atPlayhead := a.Video.PlayheadLocation(ctx)

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
		vid, atPlayhead = next, false
	}

	if !atPlayhead {
		lat, lng, _ = vid.Location()
	}
	return spot{vid: vid, lat: lat, lng: lng, atPlayhead: atPlayhead}, true
}

// state is the US state the spot is in.
//
// A per-moment coordinate is reverse-geocoded, so a clip that crosses a state
// line answers for the half on screen rather than for wherever its single fix
// happened to land. Anything less precise reads videos.state, which is that
// single fix geocoded once at ingest — reverse-geocoding it again would spend a
// Maps call to get the same answer back.
//
// Falls back to the clip's state whenever the lookup can't answer (no Maps key,
// ZERO_RESULTS). Both !guess and !state route through here, so the grading and
// the reveal can't disagree about which answer they used.
func (a *App) state(ctx context.Context, s spot) string {
	if !s.atPlayhead {
		return s.vid.State
	}
	state, err := a.Geocoder.State(s.lat, s.lng)
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
