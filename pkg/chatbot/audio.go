package chatbot

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

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
	Playing() (beds.Bed, string)
	Station() string
	PlayingAlbum() string
	RadioTrack(ctx context.Context) (artist, title string, err error)
	Pending() (beds.Switch, bool)
	Set(ctx context.Context, bed beds.Bed) error
	SetStation(ctx context.Context, station string) error
}

// bedDescs is the audience-facing name of each local bed — chat asks "what am I
// listening to", not "which enum is set". The radio isn't here: it's named by its
// tuned station, and the album fallback by whichever album the track is in —
// both known only at runtime. This is the fallback for when neither is known.
var bedDescs = map[beds.Bed]string{
	beds.CarHum: "the car's own hum",
	beds.Album:  "the music share",
}

// albumDescs credits the albums whose directory name can't carry it — an
// attribution has punctuation and a person in it, which a directory shouldn't.
// Everything else is named from its directory, so this stays a handful of
// entries rather than a catalogue that has to be fed.
var albumDescs = map[string]string{
	"fifty-horizons": "Fifty Horizons, by wooderCZ",
}

// audioCmd is the public !audio command. Anyone can ask what's playing; only
// admins switch, because the bed is the music every viewer hears at once and
// the console offers the same beds to the same person.
//
// One argument covers a bed or a selectable radio station ("!audio carhum",
// "!audio <station id>") because "switch the music to X" is one intent however
// X is spelled. Beds are matched first: a bed name is fixed vocabulary, a
// station id comes off a list that changes without a deploy.
func (a *App) audioCmd(ctx context.Context, user *users.User, params []string) {
	arg := ""
	if len(params) > 0 {
		arg = strings.ToLower(strings.TrimSpace(params[0]))
	}

	// No argument, or no permission to act on one: answer the question underneath
	// either message — what's on air — so a non-admin gets something useful
	// rather than a refusal. !song is where that answer lives.
	if arg == "" || !user.Broadcaster {
		a.songCmd(ctx, user, nil)
		return
	}

	if a.Beds == nil {
		a.Reply(ctx, "🎵 Background audio isn't wired up on this stream")
		return
	}

	var err error
	switch bed := beds.Bed(arg); {
	case beds.Valid(bed):
		err = a.Beds.Set(ctx, bed)
	case beds.SelectableStation(arg):
		err = a.Beds.SetStation(ctx, arg)
	default:
		options := append(bedNameList(), stationIDs()...)
		a.Reply(ctx, fmt.Sprintf("🎵 No background audio called %q. Options: %s",
			arg, strings.Join(options, ", ")))
		return
	}
	if err != nil {
		slog.ErrorContext(ctx, "background audio switch failed",
			"err", err, "arg", arg, "username", user.Username)
		a.Reply(ctx, "🎵 Couldn't switch the background audio right now, try again in a bit")
		return
	}

	slog.InfoContext(ctx, "background audio switched via chat", "arg", arg, "username", user.Username)
	// The switch waits a few seconds before it lands, so announcing it in the past
	// tense would name audio nobody can hear yet — and the wait is worth saying out
	// loud, since it's the window another `!audio` can correct this one in.
	if sw, ok := a.Beds.Pending(); ok {
		a.Reply(ctx, fmt.Sprintf("🎵 Switching to %s in %s",
			describeSwitch(sw), time.Until(sw.At).Round(time.Second)))
		return
	}
	a.Reply(ctx, "🎵 Switched to "+a.audioSource())
}

// describeAudio renders what's on air for chat, track first: the answer to "what
// is this song". Reads through a.Beds, so only call it with a store wired.
func (a *App) describeAudio() string {
	_, track := a.Beds.Playing()
	if t := beds.ParseTrack(track); t.Title != "" {
		return fmt.Sprintf("%q — %s", t.Title, a.audioSource())
	}
	return a.audioSource()
}

// describeSwitch names what a pending switch will land on. Reads the switch
// rather than the store, which still describes the audio that's playing.
func describeSwitch(sw beds.Switch) string {
	if sw.Bed == beds.Radio {
		return beds.StationName(sw.Station) + " on the radio"
	}
	if desc, ok := bedDescs[sw.Bed]; ok {
		return desc
	}
	return string(sw.Bed)
}

// audioSource names where the audio is coming from and nothing about the song:
// the radio bed by its channel, the album fallback by its album, the car hum by
// itself. This is what a switch announces — the track it lands on is a second
// old and about to change, so naming it there answers a question nobody asked.
func (a *App) audioSource() string {
	bed, track := a.Beds.Playing()
	if bed == beds.Radio {
		return beds.StationName(a.Beds.Station()) + " on the radio"
	}
	desc, ok := bedDescs[bed]
	if !ok {
		desc = string(bed)
	}
	if bed != beds.Album {
		return desc
	}
	// Name the album the track is actually in: the fallback covers dozens of
	// albums, and the one playing is the answer to "what is this?" — the question
	// anyone asking is asking.
	t := beds.ParseTrack(track)
	switch {
	// A tagged filename names its own album and artist, and both beat the
	// directory: the directory has to be sortable and group-prefixed
	// ("streambeats-synthwave-breaker"), so reading it aloud repeats the label the
	// track already carries and volunteers a genre nobody asked about.
	case t.Album != "" && t.Artist != "":
		return fmt.Sprintf("%s, %s", t.Album, t.Artist)
	case t.Album != "":
		return t.Album
	case a.Beds.PlayingAlbum() != "":
		return albumName(a.Beds.PlayingAlbum())
	}
	return desc
}

// albumName is an album's audience-facing name: its credit when we have one, and
// otherwise its directory read aloud — hyphens to spaces, each word capitalized,
// so "synthwave-lone-wolf" announces as "Synthwave Lone Wolf".
//
// Derived rather than tabulated because the share holds dozens of albums and
// grows without a deploy: a per-album table would be permanently one purchase
// behind, and an album missing from it would announce as a directory name.
func albumName(album string) string {
	if desc, ok := albumDescs[album]; ok {
		return desc
	}
	words := strings.Split(album, "-")
	for i, w := range words {
		if w != "" {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}

// bedNameList is the "radio, carhum" list shown when a switch names something
// that is neither a bed nor a station.
func bedNameList() []string {
	names := make([]string, len(beds.All))
	for i, b := range beds.All {
		names[i] = string(b)
	}
	return names
}

// stationIDs is the radio bed's selectable station ids, offered beside the bed
// names. Private stations are left out: chat can't select them.
func stationIDs() []string {
	var ids []string
	for _, st := range beds.StationList() {
		if !st.Private {
			ids = append(ids, st.ID)
		}
	}
	return ids
}
