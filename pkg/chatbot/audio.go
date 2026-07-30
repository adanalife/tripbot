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

// Beds is the background-audio surface !audio and !song drive: read what's on
// air, switch to another bed. It is the same *beds.Store the console's /api/audio
// drives, so a chat switch and a console switch are one switch and both report
// the same answer. Tests inject a fake; cmd/tripbot assigns the live store.
// Nil on an instance with no OBS pairing, which !audio reports rather than
// panicking on.
type Beds interface {
	Current() (beds.Bed, string)
	Station() string
	SomaFMTrack(ctx context.Context) (artist, title string, err error)
	Set(ctx context.Context, bed beds.Bed) error
	SetStation(ctx context.Context, station string) error
}

// bedDescs is the audience-facing name of each local bed — chat asks "what am I
// listening to", not "which enum is set". SomaFM isn't here: it's named by its
// selected channel, which is a runtime choice.
//
// ponytail: the album is named here because there is exactly one. Describe it
// generically (or read a title off the share) when a second album shows up —
// the same trigger that splits beds.scanTracks into per-album pools.
var bedDescs = map[beds.Bed]string{
	beds.CarHum: "the car's own hum",
	beds.Album:  "Fifty Horizons, by wooderCZ",
}

// audioCmd is the public !audio command. Anyone can ask what's playing; only
// admins switch, because the bed is the music every viewer hears at once and
// the console offers the same beds to the same person.
//
// One argument covers both a bed and a SomaFM channel ("!audio carhum",
// "!audio dronezone") because no channel id collides with a bed name, and
// "switch the music to X" is one intent however X is spelled.
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

	var err error
	switch bed := beds.Bed(arg); {
	case beds.Valid(bed):
		err = a.Beds.Set(ctx, bed)
	case beds.ValidStation(arg):
		err = a.Beds.SetStation(ctx, arg)
	default:
		// The channel list is 40-odd names, so chat gets the link rather than the
		// list — somafm.com names them better than we would anyway.
		a.Chat.Say(fmt.Sprintf("🎵 No background audio called %q. Options: %s, "+
			"or any SomaFM channel id from https://somafm.com/listen/", arg, bedNames()))
		return
	}
	if err != nil {
		slog.ErrorContext(ctx, "background audio switch failed",
			"err", err, "arg", arg, "username", user.Username)
		a.Chat.Say("🎵 Couldn't switch the background audio right now, try again in a bit")
		return
	}

	slog.InfoContext(ctx, "background audio switched via chat", "arg", arg, "username", user.Username)
	a.Chat.Say("🎵 Switched to " + a.describeAudio())
}

// describeAudio renders what's on air for chat: the SomaFM bed by its channel,
// the album by its track, the car hum by itself. Reads through a.Beds, so only
// call it with a store wired.
func (a *App) describeAudio() string {
	bed, track := a.Beds.Current()
	if bed == beds.SomaFM {
		return beds.StationName(a.Beds.Station()) + " on SomaFM"
	}
	desc, ok := bedDescs[bed]
	if !ok {
		desc = string(bed)
	}
	if title := beds.TrackTitle(track); title != "" {
		return fmt.Sprintf("%s — %q", desc, title)
	}
	return desc
}

// bedNames is the "somafm, carhum, album" list shown when a switch names
// something that is neither a bed nor a station.
func bedNames() string {
	names := make([]string, len(beds.All))
	for i, b := range beds.All {
		names[i] = string(b)
	}
	return strings.Join(names, ", ")
}
