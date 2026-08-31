package chatbot

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/adanalife/tripbot/pkg/obs/beds"
	"github.com/adanalife/tripbot/pkg/users"
)

// songCmd answers "what is this music". Which answer is right depends on the
// live background-audio bed: only SomaFM has an external now-playing feed, so
// reading that feed while the album or the car hum is on air names a track
// nobody is hearing — including when the watchdog put one of them there and
// left SomaFM selected, which is why this reads the playing bed rather than the
// selected one. Both answers come from the bed store, which is also what
// the console's now-playing line reads — one fetch of SomaFM's feed serves chat
// and every open tab.
func (a *App) songCmd(ctx context.Context, user *users.User, _ []string) {
	slog.InfoContext(ctx, "ran !song", "username", user.Username)

	if a.Beds == nil {
		a.Chat.Say("♪ Background audio isn't wired up on this stream")
		return
	}
	if bed, _ := a.Beds.Playing(); bed != beds.SomaFM {
		a.Chat.Say("♪ Now playing: " + a.describeAudio())
		return
	}

	artist, title, err := a.Beds.SomaFMTrack(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "now-playing fetch failed", "err", err,
			"station", a.Beds.Station())
		a.Chat.Say("Couldn't reach the music source for the current track, sorry!")
		return
	}
	// Naming the channel matters once there are 40-odd of them: "Drone Zone" is
	// half the answer to "what is this".
	a.Chat.Say(fmt.Sprintf("♪ Now playing on %s: %s — %s",
		beds.StationName(a.Beds.Station()), title, artist))
}
