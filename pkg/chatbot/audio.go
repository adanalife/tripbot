package chatbot

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/adanalife/tripbot/pkg/obs"
	"github.com/adanalife/tripbot/pkg/obs/beds"
	"github.com/adanalife/tripbot/pkg/users"
)

// OBS is the subset of pkg/obs the chatbot commands depend on. Tests inject a
// fake; production uses the realOBS adapter wired in New().
type OBS interface {
	RefreshBrowserSources(ctx context.Context) (int, error)
}

// realOBS delegates to pkg/obs, which dials the OBS WebSocket per call using
// the OBS_WEBSOCKET_* env vars.
type realOBS struct{}

func (realOBS) RefreshBrowserSources(ctx context.Context) (int, error) {
	return obs.RefreshBrowserSources(ctx)
}

// Beds is the background-audio surface !audio drives: read the live bed,
// switch to another. It is the same *beds.Store the console's /api/audio
// drives, so a chat switch and a console switch are one switch and both report
// the same answer. Tests inject a fake; cmd/tripbot assigns the live store.
// Nil on an instance with no OBS pairing, which !audio reports rather than
// panicking on.
type Beds interface {
	Current() (beds.Bed, string)
	Set(ctx context.Context, bed beds.Bed) error
}

// bedDescs is the audience-facing name of each bed — chat asks "what am I
// listening to", not "which enum is set".
//
// ponytail: the album is named here because there is exactly one. Describe it
// generically (or read a title off the share) when a second album shows up —
// the same trigger that splits beds.scanTracks into per-album pools.
var bedDescs = map[beds.Bed]string{
	beds.SomaFM: "Groove Salad on SomaFM",
	beds.CarHum: "the car's own hum",
	beds.Album:  "Fifty Horizons, by wooderCZ",
}

// audioCmd is the public !audio command. Anyone can ask what's playing; only
// admins switch, because the bed is the music every viewer hears at once and
// the console offers the same three beds to the same person.
func (a *App) audioCmd(ctx context.Context, user *users.User, params []string) {
	arg := ""
	if len(params) > 0 {
		arg = strings.ToLower(strings.TrimSpace(params[0]))
	}

	// No argument, or no permission to act on one: answer the question underneath
	// either message — what's on air — so a non-admin gets something useful
	// rather than a refusal. !song is where that answer lives.
	if arg == "" || !a.Cfg.UserIsAdmin(user.Username) {
		a.songCmd(ctx, user, nil)
		return
	}

	if a.Beds == nil {
		a.Chat.Say("🎵 Background audio isn't wired up on this stream")
		return
	}

	bed := beds.Bed(arg)
	if !beds.Valid(bed) {
		a.Chat.Say(fmt.Sprintf("🎵 No background audio called %q. Options: %s", arg, bedNames()))
		return
	}

	if err := a.Beds.Set(ctx, bed); err != nil {
		slog.ErrorContext(ctx, "background audio switch failed",
			"err", err, "bed", bed, "username", user.Username)
		a.Chat.Say("🎵 Couldn't switch the background audio right now, try again in a bit")
		return
	}

	slog.InfoContext(ctx, "background audio switched via chat", "bed", bed, "username", user.Username)
	a.Chat.Say("🎵 Switched to " + describeBed(a.Beds.Current()))
}

// describeBed renders a bed (and, on the album, its track) for chat. Takes the
// pair beds.Store.Current returns so callers pass it straight through.
func describeBed(bed beds.Bed, track string) string {
	desc, ok := bedDescs[bed]
	if !ok {
		desc = string(bed)
	}
	if title := beds.TrackTitle(track); title != "" {
		return fmt.Sprintf("%s — %q", desc, title)
	}
	return desc
}

// bedNames is the "somafm, carhum, album" list shown when a switch names a bed
// that doesn't exist.
func bedNames() string {
	names := make([]string, len(beds.All))
	for i, b := range beds.All {
		names[i] = string(b)
	}
	return strings.Join(names, ", ")
}
