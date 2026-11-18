package chatbot

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/adanalife/tripbot/pkg/obs/beds"
	"github.com/adanalife/tripbot/pkg/users"
)

// requestCmd implements !request <song>: queues a track of the tuned station
// to play next, ahead of its shuffle. The station's own library is all it can
// queue, so the song is matched against that by name.
func (a *App) requestCmd(ctx context.Context, user *users.User, params []string) {
	query := strings.TrimSpace(strings.Join(params, " "))
	if query == "" {
		a.Reply(ctx, "🎵 Usage: !request <song>")
		return
	}
	if a.Beds == nil {
		a.Reply(ctx, "🎵 Background audio isn't wired up on this stream")
		return
	}
	station := beds.StationName(a.Beds.Station())

	files, err := a.Beds.Tracks(ctx)
	if errors.Is(err, beds.ErrNoRequests) {
		a.Reply(ctx, fmt.Sprintf("🎵 %s doesn't take requests", station))
		return
	}
	if err != nil {
		slog.ErrorContext(ctx, "radio request: listing tracks failed", "err", err, "station", station)
		a.Reply(ctx, "🎵 Couldn't reach the radio right now, try again in a bit")
		return
	}
	file, ok := matchTrack(files, query)
	if !ok {
		a.Reply(ctx, fmt.Sprintf("🎵 Nothing on %s called %q", station, query))
		return
	}
	if err := a.Beds.Push(ctx, file); err != nil {
		slog.ErrorContext(ctx, "radio request: push failed", "err", err, "file", file, "username", user.Username)
		a.Reply(ctx, "🎵 Couldn't queue that right now, try again in a bit")
		return
	}
	slog.InfoContext(ctx, "radio request queued", "file", file, "station", station, "username", user.Username)
	a.Reply(ctx, fmt.Sprintf("🎵 Queued %q to play next on %s", trackName(file), station))
}

// matchTrack picks the file whose path contains every word of query, case
// aside; the shortest name wins a tie as the closest fit. Paths carry the
// album and artist directories, so those words match too.
func matchTrack(files []string, query string) (string, bool) {
	words := strings.Fields(strings.ToLower(query))
	best, found := "", false
	for _, f := range files {
		path := strings.ToLower(f)
		hit := true
		for _, w := range words {
			if !strings.Contains(path, w) {
				hit = false
				break
			}
		}
		if hit && (!found || len(trackName(f)) < len(trackName(best))) {
			best, found = f, true
		}
	}
	return best, found
}

// trackName is how chat names a file of the station's library: the file name
// without its extension.
func trackName(file string) string {
	return strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
}
