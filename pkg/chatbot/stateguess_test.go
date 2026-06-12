package chatbot

import "testing"

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
