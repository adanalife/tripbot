package chatbot

import "testing"

func TestFindCommand_SpacelessSplitsAtTrigger(t *testing.T) {
	cases := []struct {
		input   string
		trigger string
		params  []string
	}{
		{"!gotowyoming", "!goto", []string{"wyoming"}},
		{"!GotoWyoming", "!goto", []string{"wyoming"}},
		{"!gotowyoming now", "!goto", []string{"wyoming", "now"}},
		{"!guessflorida", "!guess", []string{"florida"}},
		// longest trigger wins: !timewarp + "back", not !time + "warpback"
		{"!timewarpback", "!timewarp", []string{"back"}},
	}
	for _, c := range cases {
		cmd, _, params := builtTestApp.findCommand(c.input)
		if cmd == nil {
			t.Errorf("findCommand(%q) = nil, want %s", c.input, c.trigger)
			continue
		}
		if cmd.Trigger != c.trigger {
			t.Errorf("findCommand(%q) trigger = %s, want %s", c.input, cmd.Trigger, c.trigger)
		}
		if len(params) != len(c.params) {
			t.Errorf("findCommand(%q) params = %v, want %v", c.input, params, c.params)
			continue
		}
		for i := range params {
			if params[i] != c.params[i] {
				t.Errorf("findCommand(%q) params = %v, want %v", c.input, params, c.params)
				break
			}
		}
	}
}

func TestFindCommand_SpacelessLeavesUnknownAlone(t *testing.T) {
	// no registered trigger prefixes these — they must stay "not found"
	for _, in := range []string{"!xyzzyfoo", "!g", "hellothere"} {
		if cmd, _, _ := builtTestApp.findCommand(in); cmd != nil {
			t.Errorf("findCommand(%q) = %s, want nil", in, cmd.Trigger)
		}
	}
}

func TestFindCommand_FuzzyBeatsSpaceless(t *testing.T) {
	// "!gotoo" is one edit from !goto — a typo, not "!goto o"
	cmd, _, params := builtTestApp.findCommand("!gotoo")
	if cmd == nil || cmd.Trigger != "!goto" {
		t.Fatalf("findCommand(!gotoo) = %v, want !goto", cmd)
	}
	if len(params) != 0 {
		t.Errorf("params = %v, want none", params)
	}
}
