package chatbot

import "testing"

func TestLevenshtein(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"abc", "abc", 0},
		{"abc", "", 3},
		{"!location", "!locaiton", 2}, // transposition = 2 plain-Levenshtein edits
		{"!state", "!stae", 1},
		{"!date", "!tate", 1},
		{"kitten", "sitting", 3},
	}
	for _, c := range cases {
		if got := levenshtein(c.a, c.b); got != c.want {
			t.Errorf("levenshtein(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestFuzzyLookup_RoutesCloseTypo(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"!locaiton", "!location"},
		{"!stae", "!state"},
		{"!comands", "!commands"},
		{"!mils", "!miles"},
		{"!shutdwon", "!shutdown"}, // routes, but the handler's own admin gate still applies
	}
	for _, c := range cases {
		cmd := builtTestApp.fuzzyLookup(c.input)
		if cmd == nil {
			t.Errorf("fuzzyLookup(%q) = nil, want %s", c.input, c.want)
			continue
		}
		if cmd.Trigger != c.want {
			t.Errorf("fuzzyLookup(%q) = %s, want %s", c.input, cmd.Trigger, c.want)
		}
	}
}

func TestFuzzyLookup_AmbiguousTieReturnsNil(t *testing.T) {
	// "!tate" is one edit from both !date and !state — refuse to guess
	if cmd := builtTestApp.fuzzyLookup("!tate"); cmd != nil {
		t.Errorf("fuzzyLookup(!tate) = %s, want nil (ambiguous)", cmd.Trigger)
	}
}

func TestFuzzyLookup_TooFarReturnsNil(t *testing.T) {
	if cmd := builtTestApp.fuzzyLookup("!zzzzzzz"); cmd != nil {
		t.Errorf("fuzzyLookup(!zzzzzzz) = %s, want nil", cmd.Trigger)
	}
}

func TestFuzzyLookup_ShortInputNeverFuzzes(t *testing.T) {
	// "!bt" is one edit from !bot, but 3-rune inputs are excluded entirely
	if cmd := builtTestApp.fuzzyLookup("!bt"); cmd != nil {
		t.Errorf("fuzzyLookup(!bt) = %s, want nil (too short)", cmd.Trigger)
	}
}

func TestFindCommand_FuzzyRoutesWithParams(t *testing.T) {
	cmd, params := builtTestApp.findCommand("!gotoo 42")
	if cmd == nil {
		t.Fatal("expected a command, got nil")
	}
	if cmd.Trigger != "!goto" {
		t.Errorf("got trigger %q, want !goto", cmd.Trigger)
	}
	if len(params) != 1 || params[0] != "42" {
		t.Errorf("unexpected params: %v", params)
	}
}

func TestFindCommand_BareWordNotFuzzyRouted(t *testing.T) {
	// bare-word triggers ("hello") are only reachable by exact match
	if cmd, _ := builtTestApp.findCommand("helo"); cmd != nil {
		t.Errorf("findCommand(helo) = %s, want nil", cmd.Trigger)
	}
}
