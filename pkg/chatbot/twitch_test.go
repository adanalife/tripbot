package chatbot

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/adanalife/tripbot/pkg/video"
)

// noopTwitch satisfies Twitch for tests that don't care about follow lookups.
type noopTwitch struct{}

func (noopTwitch) FollowedAt(string) (time.Time, bool) { return time.Time{}, false }

// recordingTwitch captures FollowedAt lookups and returns a staged result so
// tests can assert both the username queried and the rendered reply.
type recordingTwitch struct {
	Result  time.Time
	OK      bool
	Lookups []string
}

func (r *recordingTwitch) FollowedAt(username string) (time.Time, bool) {
	r.Lookups = append(r.Lookups, username)
	return r.Result, r.OK
}

func TestFollowageCmd_Following_RepliesWithDuration(t *testing.T) {
	app := newTestApp(video.Video{})
	rec := &recordingChat{}
	app.Chat = rec
	app.Twitch = &recordingTwitch{Result: time.Now().Add(-48 * time.Hour), OK: true}

	app.followageCmd(context.Background(), newTestUser("viewer1"), nil)

	if got := strings.Join(rec.Says, " "); !strings.Contains(got, "following for") {
		t.Fatalf("expected a following-duration reply, got %q", rec.Says)
	}
}

func TestFollowageCmd_NotFollowing_SelfPrompt(t *testing.T) {
	app := newTestApp(video.Video{})
	rec := &recordingChat{}
	app.Chat = rec
	app.Twitch = &recordingTwitch{OK: false}

	app.followageCmd(context.Background(), newTestUser("viewer1"), nil)

	if got := strings.Join(rec.Says, " "); !strings.Contains(got, "follow button") {
		t.Fatalf("expected the not-following self prompt, got %q", rec.Says)
	}
}

func TestFollowageCmd_OtherUser_LooksUpStrippedName(t *testing.T) {
	app := newTestApp(video.Video{})
	rec := &recordingChat{}
	app.Chat = rec
	twitch := &recordingTwitch{OK: false}
	app.Twitch = twitch

	app.followageCmd(context.Background(), newTestUser("viewer1"), []string{"@someoneElse"})

	if len(twitch.Lookups) != 1 || twitch.Lookups[0] != "someoneElse" {
		t.Fatalf("expected a lookup for the @-stripped target, got %v", twitch.Lookups)
	}
	if got := strings.Join(rec.Says, " "); !strings.Contains(got, "isn't following") {
		t.Fatalf("expected the other-user not-following reply, got %q", rec.Says)
	}
}
