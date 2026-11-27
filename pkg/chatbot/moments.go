package chatbot

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/adanalife/tripbot/pkg/users"
	"github.com/adanalife/tripbot/pkg/video"
)

// maxMarkNoteRunes matches the console's cap on a moment's note.
const maxMarkNoteRunes = 200

// trimRollover is how soon after a clip starts a !trim is taken to mean the
// clip that just left the screen too: a viewer reacts after the bad stretch.
const trimRollover = 5 * time.Second

// favoriteCmd stars the moment on screen for the viewer, the same mark the
// console's ⭐ sets, with the rest of the line as its note.
func (a *App) favoriteCmd(ctx context.Context, user *users.User, params []string) {
	a.markMoment(ctx, user, "star", params, "⭐ starred %s of this clip, thanks @%s!")
}

// trimCmd records the viewer's report that the stretch on screen should be
// cut, as a trim mark the console lists beside the clip's other marks.
func (a *App) trimCmd(ctx context.Context, user *users.User, params []string) {
	a.markMoment(ctx, user, "trim", params, "✂️ noted %s of this clip for a trim, thanks @%s!")
}

// markMoment marks the playhead with kind for user and replies with reply,
// formatted with the clip's m:ss and the user's name.
func (a *App) markMoment(ctx context.Context, user *users.User, kind string, params []string, reply string) {
	slog.InfoContext(ctx, "ran a moment mark", "username", user.Username, "kind", kind)
	vid, elapsed := a.Video.Playhead(ctx)
	if vid.ID == 0 {
		a.Reply(ctx, "I can't tell what's on screen right now, sorry!")
		return
	}
	note := []rune(strings.TrimSpace(strings.Join(params, " ")))
	if len(note) > maxMarkNoteRunes {
		note = note[:maxMarkNoteRunes]
	}
	err := a.Video.SetMark(ctx, video.Mark{VideoID: vid.ID, Kind: kind, Sec: elapsed.Seconds(),
		Note: string(note), By: user.Username, Platform: a.platform()})
	if err != nil {
		slog.ErrorContext(ctx, "error marking a moment", "err", err,
			"username", user.Username, "kind", kind, "video_id", vid.ID)
		a.Reply(ctx, "I couldn't save that, sorry!")
		return
	}
	if kind == "trim" && elapsed < trimRollover {
		a.markPreviousEnd(ctx, user, string(note))
	}
	secs := int(elapsed.Seconds())
	a.Reply(ctx, fmt.Sprintf(reply, fmt.Sprintf("%d:%02d", secs/60, secs%60), user.Username))
}

// markPreviousEnd marks the last aired second of the clip before the one on
// screen as a trim too. Best effort: the reply is about the clip on screen.
func (a *App) markPreviousEnd(ctx context.Context, user *users.User, note string) {
	id, aired, ok, err := a.Video.PreviousPlay(ctx, a.platform())
	if err == nil && ok {
		err = a.Video.SetMark(ctx, video.Mark{VideoID: id, Kind: "trim", Sec: aired,
			Note: note, By: user.Username, Platform: a.platform()})
	}
	if err != nil {
		slog.ErrorContext(ctx, "error marking the previous clip", "err", err, "username", user.Username)
	}
}
