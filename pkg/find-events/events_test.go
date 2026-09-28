package findEvents

import "testing"

func TestRunSubject(t *testing.T) {
	if got, want := RunSubject("prod", "twitch"), "tripbot.prod.find.run.twitch"; got != want {
		t.Errorf("RunSubject = %q, want %q", got, want)
	}
}
