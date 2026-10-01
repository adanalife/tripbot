package chatbot

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"github.com/adanalife/tripbot/pkg/feature"
	"github.com/adanalife/tripbot/pkg/gateway"
	"github.com/adanalife/tripbot/pkg/helpers"
)

// streamTitleFlagKey gates writing the stream title from the footage on
// screen. Off, the title is whatever the operator last saved.
const streamTitleFlagKey = "chatbot.stream_title"

// StreamMetadata is the platform-gateway's broadcast-metadata store: the title,
// tags and category the gateway applies to the platform and re-applies at
// every go-live. Writing through it rather than to the platform directly keeps
// the store the one source of truth, so the gateway's drift alert stays quiet.
// Nil on an instance with no gateway, which leaves the title alone.
type StreamMetadata interface {
	StoredMetadata(ctx context.Context) (gateway.Metadata, bool, error)
	SetMetadata(ctx context.Context, m gateway.Metadata) error
}

// UpdateStreamTitle sets the stream title to where the van is and the light it
// was filmed in — "Driving through Bishop, California at golden hour".
//
// It runs on a timer and writes only when the title would change, so the
// timer's interval is the most often a title can change. Only the title is
// replaced; the stored tags, category and description go back as they were.
// While the flag is on the operator's own title is overwritten on the next
// change, which is the point: the flag is how they take it back.
func (a *App) UpdateStreamTitle(ctx context.Context) {
	if a.Metadata == nil {
		return
	}
	if !a.Flags.Bool(ctx, streamTitleFlagKey, feature.EvalContext{
		Channel: a.Cfg.ChannelName,
		Env:     a.Cfg.Environment,
	}) {
		return
	}
	title := a.streamTitle(ctx)
	if title == "" {
		return
	}

	m, _, err := a.Metadata.StoredMetadata(ctx)
	if err != nil {
		a.logTitleErr(ctx, "stream title: stored metadata unreadable", err)
		return
	}
	if m.Title == title {
		return
	}
	m.Title = title
	if err := a.Metadata.SetMetadata(ctx, m); err != nil {
		a.logTitleErr(ctx, "stream title: write failed", err)
		return
	}
	slog.InfoContext(ctx, "stream title set", "title", title)
}

// streamTitle renders the title for the moment on screen, or "" when there is
// nothing honest to say — a flagged clip has no usable GPS, and the last title
// written is a better answer than a guess.
//
// The place comes only from what the pipeline resolved offline; a live geocode
// every few minutes would spend a Maps call to rename a title nobody asked for.
func (a *App) streamTitle(ctx context.Context) string {
	vid, at, tracked := a.Video.PlayheadLocation(ctx)
	if vid.Flagged {
		return ""
	}
	place := at.Place()
	if place == "" {
		place = vid.Place()
	}
	if place == "" {
		return ""
	}

	switch {
	case strings.HasPrefix(place, "Somewhere in "):
		place = "Driving through " + strings.TrimPrefix(place, "Somewhere in ")
	case strings.HasPrefix(place, "near "):
		place = "Driving " + place
	default:
		place = "Driving through " + place
	}
	if vid.DateFilmed.IsZero() {
		return place
	}
	lat, lng := vid.Lat, vid.Lng
	if tracked {
		lat, lng = at.Lat, at.Lng
	}
	return place + " " + helpers.DayPart(vid.DateFilmed, lat, lng)
}

// logTitleErr keeps a throttled or briefly unreachable store off Sentry: the
// next tick retries, so only an unexpected refusal is an error.
func (a *App) logTitleErr(ctx context.Context, msg string, err error) {
	if errors.Is(err, gateway.ErrUpstreamUnavailable) {
		slog.WarnContext(ctx, msg, "err", err)
		return
	}
	slog.ErrorContext(ctx, msg, "err", err)
}
