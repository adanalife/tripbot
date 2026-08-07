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
	Current() (beds.Bed, string)
	Station() string
	Album() string
	PlayingAlbum() string
	Albums() []string
	Groups() []string
	ResolveAlbum(arg string) string
	SomaFMTrack(ctx context.Context) (artist, title string, err error)
	Pending() (beds.Switch, bool)
	Set(ctx context.Context, bed beds.Bed) error
	SetStation(ctx context.Context, station string) error
	SetAlbum(ctx context.Context, album string) error
}

// bedDescs is the audience-facing name of each local bed — chat asks "what am I
// listening to", not "which enum is set". SomaFM isn't here: it's named by its
// tuned channel, and the album bed by whichever album the track is in — both
// runtime choices. This is the fallback for when neither is known.
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
// One argument covers a bed, a SomaFM channel, an album on the share, or a group
// of albums ("!audio carhum", "!audio dronezone", "!audio rose", "!audio
// streambeats") because none of those namespaces collide, and "switch the music
// to X" is one intent however X is spelled. Beds are matched first and anything
// off the share last: a bed name is fixed vocabulary, a directory is something
// Dana can rename, so the share can never shadow a word the command already
// answered to.
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
		album := a.Beds.ResolveAlbum(arg)
		if album == "" {
			// The channel list is 40-odd names, so chat gets the link rather than the
			// list — somafm.com names them better than we would anyway. The albums
			// are ours and few, so those are named outright.
			// Groups first: "streambeats" is a more useful thing to be told about
			// than any one of the 29 albums under it.
			options := append(bedNameList(), a.Beds.Groups()...)
			a.Chat.Say(fmt.Sprintf("🎵 No background audio called %q. Options: %s, "+
				"any album on the share, or any SomaFM channel id from "+
				"https://somafm.com/listen/", arg, strings.Join(options, ", ")))
			return
		}
		err = a.Beds.SetAlbum(ctx, album)
	}
	if err != nil {
		slog.ErrorContext(ctx, "background audio switch failed",
			"err", err, "arg", arg, "username", user.Username)
		a.Chat.Say("🎵 Couldn't switch the background audio right now, try again in a bit")
		return
	}

	slog.InfoContext(ctx, "background audio switched via chat", "arg", arg, "username", user.Username)
	// The switch waits a few seconds before it lands, so announcing it in the past
	// tense would name audio nobody can hear yet — and the wait is worth saying out
	// loud, since it's the window another `!audio` can correct this one in.
	if sw, ok := a.Beds.Pending(); ok {
		a.Chat.Say(fmt.Sprintf("🎵 Switching to %s in %s",
			describeSwitch(sw), time.Until(sw.At).Round(time.Second)))
		return
	}
	a.Chat.Say("🎵 Switched to " + a.audioSource())
}

// describeAudio renders what's on air for chat, track first: the answer to "what
// is this song". Reads through a.Beds, so only call it with a store wired.
func (a *App) describeAudio() string {
	_, track := a.Beds.Current()
	if t := beds.ParseTrack(track); t.Title != "" {
		return fmt.Sprintf("%q — %s", t.Title, a.audioSource())
	}
	return a.audioSource()
}

// describeSwitch names what a pending switch will land on. Reads the switch
// rather than the store, which still describes the audio that's playing — and on
// the album bed it can only name the selection, since the track it will start on
// hasn't been chosen yet.
func describeSwitch(sw beds.Switch) string {
	switch sw.Bed {
	case beds.SomaFM:
		return beds.StationName(sw.Station) + " on SomaFM"
	case beds.Album:
		if sw.Album == "" {
			return "everything on the music share"
		}
		return albumName(sw.Album)
	}
	if desc, ok := bedDescs[sw.Bed]; ok {
		return desc
	}
	return string(sw.Bed)
}

// audioSource names where the audio is coming from and nothing about the song:
// the SomaFM bed by its channel, the album bed by its album, the car hum by
// itself. This is what a switch announces — the track it lands on is a second
// old and about to change, so naming it there answers a question nobody asked.
func (a *App) audioSource() string {
	bed, track := a.Beds.Current()
	if bed == beds.SomaFM {
		return beds.StationName(a.Beds.Station()) + " on SomaFM"
	}
	desc, ok := bedDescs[bed]
	if !ok {
		desc = string(bed)
	}
	if bed != beds.Album {
		return desc
	}
	// Name the album the track is actually in rather than the selection: on a group
	// ("streambeats-lofi") or the whole share the selection covers dozens of
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
	case a.Beds.Album() != "":
		return albumName(a.Beds.Album())
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

// bedNameList is the "somafm, carhum, album" list shown when a switch names
// something that is neither a bed, a station, nor an album.
func bedNameList() []string {
	names := make([]string, len(beds.All))
	for i, b := range beds.All {
		names[i] = string(b)
	}
	return names
}
