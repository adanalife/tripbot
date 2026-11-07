package chatbot

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/adanalife/tripbot/pkg/video"
)

func TestMarkMoment(t *testing.T) {
	ctx := context.Background()
	app := newTestApp(video.Video{})
	v := &recordingVideo{Vid: video.Video{ID: 7}, Elapsed: 192500 * time.Millisecond}
	app.Video = v
	app.Platform = "youtube"
	out, _ := captureSay(t, app)

	app.favoriteCmd(ctx, newTestUser("ada"), []string{"the", "canyon", "opens", "up"})
	app.trimCmd(ctx, newTestUser("bo"), nil)
	app.boringCmd(ctx, newTestUser("cy"), []string{"nothing", "happens"})

	want := []video.Mark{
		{VideoID: 7, Kind: "star", Sec: 192.5, Note: "the canyon opens up", By: "ada", Platform: "youtube"},
		{VideoID: 7, Kind: "trim", Sec: 192.5, By: "bo", Platform: "youtube"},
		{VideoID: 7, Kind: "boring", Sec: 192.5, ClipLevel: true, Note: "nothing happens", By: "cy", Platform: "youtube"},
	}
	if len(v.Marks) != 3 || v.Marks[0] != want[0] || v.Marks[1] != want[1] || v.Marks[2] != want[2] {
		t.Errorf("marks = %+v, want %+v", v.Marks, want)
	}
	if got := out(); !strings.Contains(got, "starred 3:12") || !strings.Contains(got, "3:12 of this clip for a trim, thanks @bo") ||
		!strings.Contains(got, "noted all of this clip as boring, thanks @cy") {
		t.Errorf("replies = %q", got)
	}

	app.favoriteCmd(ctx, newTestUser("ada"), []string{strings.Repeat("é", maxMarkNoteRunes+5)})
	if got := []rune(v.Marks[3].Note); len(got) != maxMarkNoteRunes {
		t.Errorf("note kept %d runes, want %d", len(got), maxMarkNoteRunes)
	}
	out()

	v.MarkErr = errors.New("db down")
	app.trimCmd(ctx, newTestUser("bo"), nil)
	if got := out(); !strings.Contains(got, "couldn't save") {
		t.Errorf("failed write replied %q", got)
	}

	v.Vid = video.Video{}
	app.trimCmd(ctx, newTestUser("bo"), nil)
	if got := out(); !strings.Contains(got, "can't tell what's on screen") || len(v.Marks) != 4 {
		t.Errorf("no clip: replied %q, %d marks", got, len(v.Marks))
	}
}
