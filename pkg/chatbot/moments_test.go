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

	want := []video.Mark{
		{VideoID: 7, Kind: "star", Sec: 192.5, Note: "the canyon opens up", By: "ada", Platform: "youtube"},
		{VideoID: 7, Kind: "trim", Sec: 192.5, By: "bo", Platform: "youtube"},
	}
	if len(v.Marks) != 2 || v.Marks[0] != want[0] || v.Marks[1] != want[1] {
		t.Errorf("marks = %+v, want %+v", v.Marks, want)
	}
	if got := out(); !strings.Contains(got, "starred 3:12") || !strings.Contains(got, "3:12 of this clip for a trim, thanks @bo") {
		t.Errorf("replies = %q", got)
	}

	app.favoriteCmd(ctx, newTestUser("ada"), []string{strings.Repeat("é", maxMarkNoteRunes+5)})
	if got := []rune(v.Marks[2].Note); len(got) != maxMarkNoteRunes {
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
	if got := out(); !strings.Contains(got, "can't tell what's on screen") || len(v.Marks) != 3 {
		t.Errorf("no clip: replied %q, %d marks", got, len(v.Marks))
	}
}

// A trim in the first seconds of a clip also marks where the clip before it
// left the screen; later, and for stars, it doesn't.
func TestTrimAtRolloverMarksThePreviousClip(t *testing.T) {
	ctx := context.Background()
	app := newTestApp(video.Video{})
	v := &recordingVideo{Vid: video.Video{ID: 8}, Elapsed: 3 * time.Second, PrevID: 7, PrevAired: 61.5}
	app.Video = v
	app.Platform = "twitch"
	out, _ := captureSay(t, app)

	app.trimCmd(ctx, newTestUser("bo"), []string{"fog"})
	app.favoriteCmd(ctx, newTestUser("bo"), nil)
	v.Elapsed = 9 * time.Second
	app.trimCmd(ctx, newTestUser("ada"), nil)
	out()

	want := []video.Mark{
		{VideoID: 8, Kind: "trim", Sec: 3, Note: "fog", By: "bo", Platform: "twitch"},
		{VideoID: 7, Kind: "trim", Sec: 61.5, Note: "fog", By: "bo", Platform: "twitch"},
		{VideoID: 8, Kind: "star", Sec: 3, By: "bo", Platform: "twitch"},
		{VideoID: 8, Kind: "trim", Sec: 9, By: "ada", Platform: "twitch"},
	}
	if len(v.Marks) != len(want) {
		t.Fatalf("marks = %+v, want %+v", v.Marks, want)
	}
	for i := range want {
		if v.Marks[i] != want[i] {
			t.Errorf("mark %d = %+v, want %+v", i, v.Marks[i], want[i])
		}
	}
}
