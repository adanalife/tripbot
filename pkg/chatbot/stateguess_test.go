package chatbot

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestFindCommand_StateNameRoutesToGuess(t *testing.T) {
	cmd, params := builtTestApp.findCommand("!florida")
	if cmd == nil {
		t.Fatal("expected a command, got nil")
	}
	if cmd.Trigger != "!guess" {
		t.Errorf("got trigger %q, want !guess", cmd.Trigger)
	}
	if len(params) != 1 || params[0] != "florida" {
		t.Errorf("unexpected params: %v", params)
	}
}

func TestFindCommand_MultiWordStateRoutesToGuess(t *testing.T) {
	cmd, params := builtTestApp.findCommand("!new york")
	if cmd == nil {
		t.Fatal("expected a command, got nil")
	}
	if cmd.Trigger != "!guess" {
		t.Errorf("got trigger %q, want !guess", cmd.Trigger)
	}
	if len(params) != 2 || params[0] != "new" || params[1] != "york" {
		t.Errorf("unexpected params: %v", params)
	}
}

func TestFindCommand_StateWithTrailingTextRoutesToGuess(t *testing.T) {
	// trailing chatter after a state name is dropped from the guess
	cmd, params := builtTestApp.findCommand("!florida woo")
	if cmd == nil {
		t.Fatal("expected a command, got nil")
	}
	if cmd.Trigger != "!guess" {
		t.Errorf("got trigger %q, want !guess", cmd.Trigger)
	}
	if len(params) != 1 || params[0] != "florida" {
		t.Errorf("unexpected params: %v", params)
	}
}

func TestFindCommand_StateAbbrevDoesNotRoute(t *testing.T) {
	// two-letter abbreviations are deliberately excluded ("!hi", "!ok",
	// "!me" would fire accidental guesses)
	for _, token := range []string{"!fl", "!hi", "!ok"} {
		if cmd, _ := builtTestApp.findCommand(token); cmd != nil {
			t.Errorf("findCommand(%q) = %s, want nil", token, cmd.Trigger)
		}
	}
}

func TestFindCommand_StateShortcutDisabledOnYouTube(t *testing.T) {
	// !guess isn't in the YouTube allowlist, so the shortcut must not fire
	yt := &App{Platform: platformYouTube}
	yt.indexCommands()
	if cmd, _ := yt.findCommand("!florida"); cmd != nil {
		t.Errorf("findCommand(!florida) on YouTube = %s, want nil", cmd.Trigger)
	}
}

func TestStateGuessParams_NonState(t *testing.T) {
	if got := stateGuessParams("!notastate", nil); got != nil {
		t.Errorf("stateGuessParams(!notastate) = %v, want nil", got)
	}
}

func TestFuzzyStateName(t *testing.T) {
	cases := []struct {
		guess string
		want  string
	}{
		{"florisa", "Florida"},
		{"califonia", "California"},
		{"new yrok", "New York"}, // transposition, but long enough for 2 edits
		{"utak", "Utah"},
		{"utha", ""},            // transposition = 2 edits; short inputs only get 1 (known limitation)
		{"arkansa", "Arkansas"}, // distance 1 beats Kansas at 2 — no tie
		{"florida", ""},         // exact names are never touched
		{"FLORIDA", ""},         // ...case-insensitively
		{"xyzzy", ""},           // nowhere near a state
		{"fl", ""},              // too short to fuzz (abbrevs handled upstream)
		{"", ""},
	}
	for _, c := range cases {
		if got := fuzzyStateName(c.guess); got != c.want {
			t.Errorf("fuzzyStateName(%q) = %q, want %q", c.guess, got, c.want)
		}
	}
}

func TestGuessCmd_CorrectGuess_Misspelled(t *testing.T) {
	// a close misspelling of the right state still wins
	mock := installMockDB(t)
	vid := newTestVideo("Massachusetts", 42.3, -71.0, time.Now())
	app := newTestApp(vid)

	expectAddToScoreChain(mock)
	expectAddToScoreChain(mock)

	out := captureSay(t, app)

	app.guessCmd(context.Background(), newTestUser("viewer1"), []string{"massachusets"})

	if !strings.Contains(out(), "got it") {
		t.Errorf("expected correct-guess msg, got %q", out())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestGuessCmd_WrongGuess_MisspelledStaysWrong(t *testing.T) {
	// a misspelling of the WRONG state corrects to that state and stays wrong
	vid := newTestVideo("Colorado", 39.5, -105.0, time.Now())
	app := newTestApp(vid)
	out := captureSay(t, app)

	app.guessCmd(context.Background(), newTestUser("viewer1"), []string{"wyomig"})

	if !strings.Contains(out(), "Try again") {
		t.Errorf("expected try-again in output, got %q", out())
	}
}
