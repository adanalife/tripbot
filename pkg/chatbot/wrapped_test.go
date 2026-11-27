package chatbot

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/adanalife/tripbot/pkg/scoreboards"
	"github.com/adanalife/tripbot/pkg/video"
)

var october = time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC)

func TestWrappedLine(t *testing.T) {
	for name, tc := range map[string]struct {
		w    scoreboards.Wrapped
		want string
	}{
		"full month": {
			scoreboards.Wrapped{Month: october, Guesses: 7, Correct: 3, States: 2, Rank: 2, Players: 3,
				MissedMost: "Vermont", CalledIt: "New Hampshire", Minutes: 90},
			"@alice, your October: 7 guesses, 3 right (#2 of 3 on the board), 2 different states called; Vermont gets you most — you keep saying New Hampshire. 1h 30m in the van with me 🚐",
		},
		"nothing at all": {
			scoreboards.Wrapped{Month: october},
			"@alice, nothing from you in October yet — !guess a state and come back!",
		},
		"watched but never guessed": {
			scoreboards.Wrapped{Month: october, Minutes: 45},
			"@alice, your October: no guesses. 45m in the van with me 🚐",
		},
		"one guess, off the board, one state": {
			scoreboards.Wrapped{Month: october, Guesses: 1, Correct: 1, States: 1, Players: 9, Minutes: 180},
			"@alice, your October: 1 guess, 1 right. 3h in the van with me 🚐",
		},
		"missed state called by a non-state": {
			scoreboards.Wrapped{Month: october, Guesses: 2, Correct: 0, MissedMost: "Idaho", CalledIt: "idaho"},
			"@alice, your October: 2 guesses, 0 right; Idaho gets you most",
		},
	} {
		if got := wrappedLine("alice", tc.w); got != tc.want {
			t.Errorf("%s:\n got %q\nwant %q", name, got, tc.want)
		}
	}
}

func newWrappedTestApp(t *testing.T, sb *recordingScoreboards) (*App, *recordingChat) {
	t.Helper()
	app := newTestApp(video.Video{})
	recChat := &recordingChat{}
	app.Chat = recChat
	app.Scoreboards = sb
	return app, recChat
}

func TestWrappedCmd_RepliesWithThisMonth(t *testing.T) {
	sb := &recordingScoreboards{WrappedStats: scoreboards.Wrapped{Month: october, Guesses: 4, Correct: 4, States: 3}}
	app, recChat := newWrappedTestApp(t, sb)

	app.wrappedCmd(context.Background(), newAdminUser(), nil)

	if len(sb.Calls) != 1 || !strings.HasPrefix(sb.Calls[0], `Wrapped("test", `+time.Now().UTC().Format("2006-01")) {
		t.Errorf("expected one Wrapped read for this month, got %v", sb.Calls)
	}
	if got := recChat.Output(); !strings.Contains(got, "4 guesses, 4 right, 3 different states called") {
		t.Errorf("unexpected reply %q", got)
	}
}

func TestWrappedCmd_LastReadsThePreviousMonth(t *testing.T) {
	sb := &recordingScoreboards{Last: time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC)}
	app, _ := newWrappedTestApp(t, sb)

	app.wrappedCmd(context.Background(), newAdminUser(), []string{"LAST"})

	if len(sb.Calls) != 1 || sb.Calls[0] != `Wrapped("test", 2026-09)` {
		t.Errorf("expected a read for September, got %v", sb.Calls)
	}
}

func TestWrappedCmd_ReadError(t *testing.T) {
	sb := &recordingScoreboards{WrappedErr: errors.New("db down")}
	app, recChat := newWrappedTestApp(t, sb)

	app.wrappedCmd(context.Background(), newAdminUser(), nil)

	if got := recChat.Output(); !strings.Contains(got, "Can't add that up") {
		t.Errorf("expected the error reply, got %q", got)
	}
}
