package scoreboards

import (
	"regexp"
	"testing"
)

var monthlySuffix = regexp.MustCompile(`^[a-z_]+_\d{4}_\d{2}$`)

func TestCurrentMilesScoreboardFormat(t *testing.T) {
	got := CurrentMilesScoreboard()
	if !monthlySuffix.MatchString(got) {
		t.Fatalf("got %q, want match for %s", got, monthlySuffix)
	}
	if got[:len("miles_")] != "miles_" {
		t.Fatalf("expected miles_ prefix, got %q", got)
	}
}

func TestCurrentGuessScoreboardFormat(t *testing.T) {
	got := CurrentGuessScoreboard()
	if !monthlySuffix.MatchString(got) {
		t.Fatalf("got %q, want match for %s", got, monthlySuffix)
	}
	if got[:len("guess_state_")] != "guess_state_" {
		t.Fatalf("expected guess_state_ prefix, got %q", got)
	}
}
