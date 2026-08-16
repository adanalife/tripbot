package chatbot

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestFindCommand_StateNameRoutesToGuess(t *testing.T) {
	cmd, _, params := builtTestApp.findCommand("!florida")
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
	cmd, _, params := builtTestApp.findCommand("!new york")
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
	cmd, _, params := builtTestApp.findCommand("!florida woo")
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
		if cmd, _, _ := builtTestApp.findCommand(token); cmd != nil {
			t.Errorf("findCommand(%q) = %s, want nil", token, cmd.Trigger)
		}
	}
}

func TestFindCommand_StateShortcutDisabledOnYouTube(t *testing.T) {
	// !guess isn't in the YouTube allowlist, so the shortcut must not fire
	yt := &App{Platform: platformYouTube}
	yt.indexCommands()
	if cmd, _, _ := yt.findCommand("!florida"); cmd != nil {
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
	vid := newTestVideo("Massachusetts", 42.3, -71.0, time.Now())
	app := newTestApp(vid)
	recScores := &recordingScoreboards{}
	app.Scoreboards = recScores

	out := captureSay(t, app)

	app.guessCmd(context.Background(), newTestUser("viewer1"), []string{"massachusets"})

	if !strings.Contains(out(), "got it") {
		t.Errorf("expected correct-guess msg, got %q", out())
	}
	// A fuzzy-corrected guess scores like an exact one.
	if len(recScores.Credited) != 1 {
		t.Errorf("credited = %v, want the guesser once", recScores.Credited)
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

// An unflagged video can still have no state: pkg/video's save path only
// geocodes when coords parse, and a geocoder error (including the expected
// ErrDisabled when no Maps key is set) leaves State empty without flagging.
// Nobody can guess an answer that doesn't exist, so nobody gets credited.
func TestGuessCmd_StatelessVideo_CreditsNobody(t *testing.T) {
	// "zz" is the two-letter case: it must not be blanked before the comparison,
	// or "" == "" matches the stateless video and hands out a guess point plus a
	// timewarp on demand.
	for _, guess := range []string{"zz", "Colorado", ""} {
		t.Run("guess="+guess, func(t *testing.T) {
			app := newTestApp(newTestVideo("", 39.5, -105.0, time.Now()))
			recScores := &recordingScoreboards{}
			recPlayout := &recordingPlayout{}
			app.Scoreboards = recScores
			app.Playout = recPlayout
			out := captureSay(t, app)

			app.guessCmd(context.Background(), newTestUser("viewer1"), []string{guess})

			if strings.Contains(out(), "got it") {
				t.Errorf("guess %q won against a stateless video: %q", guess, out())
			}
			if len(recScores.Credited) != 0 {
				t.Errorf("credited = %v, want nobody", recScores.Credited)
			}
			if len(recPlayout.Calls) != 0 {
				t.Errorf("playout calls = %v, want no timewarp", recPlayout.Calls)
			}
		})
	}
}

// A two-letter guess that isn't a state code survives to the comparison as
// typed instead of being blanked, matching what pkg/video's FindRandomByState
// does with the same lookup.
func TestGuessCmd_TwoLetterNonState_IsNotBlanked(t *testing.T) {
	app := newTestApp(newTestVideo("Colorado", 39.5, -105.0, time.Now()))
	recScores := &recordingScoreboards{}
	app.Scoreboards = recScores
	out := captureSay(t, app)

	app.guessCmd(context.Background(), newTestUser("viewer1"), []string{"zz"})

	if !strings.Contains(out(), "Try again") {
		t.Errorf("expected try-again for a non-state pair, got %q", out())
	}
	if len(recScores.Credited) != 0 {
		t.Errorf("credited = %v, want nobody", recScores.Credited)
	}
}

// Wrong guesses after the first carry a warmer/colder hint: the EarthDay emote
// is swapped for 🔥 when this guess's state is closer to the van than the
// chatter's previous one, ❄️ when it's farther.
func TestGuessCmd_WrongGuess_WarmerColderHints(t *testing.T) {
	// Pin the round start in the past and restore it after: entries stamped
	// before lastTimewarpTime belong to a previous round.
	saved := lastTimewarpTime
	lastTimewarpTime = time.Now().Add(-time.Hour)
	t.Cleanup(func() { lastTimewarpTime = saved })

	app := newTestApp(newTestVideo("Colorado", 39.5, -105.0, time.Now()))
	out := captureSay(t, app)
	user := newTestUser("viewer1")

	// First miss: no previous distance, no hint.
	app.guessCmd(context.Background(), user, []string{"Florida"})
	if got := out(); !strings.Contains(got, "EarthDay") {
		t.Errorf("first miss should show EarthDay, got %q", got)
	}

	// A guess with no centroid can't be measured: no hint, and it doesn't
	// disturb the trail — the next miss still compares against Florida.
	app.guessCmd(context.Background(), user, []string{"Guam"})
	if got := out(); !strings.Contains(got, "EarthDay") {
		t.Errorf("centroid-less miss should show EarthDay, got %q", got)
	}

	// Wyoming is closer to Colorado than Florida: warmer.
	app.guessCmd(context.Background(), user, []string{"Wyoming"})
	if got := out(); !strings.Contains(got, "🔥") {
		t.Errorf("closer miss should show fire, got %q", got)
	}

	// Another chatter's first miss gets no hint — trails are per-user.
	app.guessCmd(context.Background(), newTestUser("viewer2"), []string{"Utah"})
	if got := out(); !strings.Contains(got, "EarthDay") {
		t.Errorf("another chatter's first miss should show EarthDay, got %q", got)
	}

	// Florida again is farther than Wyoming: colder.
	app.guessCmd(context.Background(), user, []string{"Florida"})
	if got := out(); !strings.Contains(got, "❄️") {
		t.Errorf("farther miss should show snowflake, got %q", got)
	}

	// The same state twice is the same distance: nothing to compare.
	app.guessCmd(context.Background(), user, []string{"Florida"})
	if got := out(); !strings.Contains(got, "EarthDay") {
		t.Errorf("equal-distance miss should show EarthDay, got %q", got)
	}

	// A timewarp starts a new round: the trail is stale, so no hint.
	lastTimewarpTime = time.Now()
	app.guessCmd(context.Background(), user, []string{"Nevada"})
	if got := out(); !strings.Contains(got, "EarthDay") {
		t.Errorf("first miss after a timewarp should show EarthDay, got %q", got)
	}
}
