package rotator

import (
	"errors"
	"strings"
	"testing"
)

func TestSanitizeNormalizesEditorArtifacts(t *testing.T) {
	got, err := Sanitize(Config{
		Left: Corner{Messages: []Message{
			{Text: "  padded  "},
			{Text: "   "}, // blank row from the editor
			{Text: "negative", Weight: -5},
			{Text: "scoped", Platforms: []string{PlatformTwitch}},
		}},
		RareMessage: "  rare  ",
	})
	if err != nil {
		t.Fatalf("Sanitize: %v", err)
	}
	msgs := got.Left.Messages
	if len(msgs) != 3 {
		t.Fatalf("got %d messages, want 3 (the blank row dropped): %+v", len(msgs), msgs)
	}
	if msgs[0].Text != "padded" {
		t.Errorf("text = %q, want it trimmed", msgs[0].Text)
	}
	if msgs[1].Weight != 0 {
		t.Errorf("negative weight = %d, want 0", msgs[1].Weight)
	}
	if msgs[2].Platforms != nil {
		t.Errorf("Platforms = %v, want cleared (stored copy is per-platform)", msgs[2].Platforms)
	}
	if got.RareMessage != "rare" {
		t.Errorf("rare message = %q, want it trimmed", got.RareMessage)
	}
}

// The whole reason length validation lives next to the budgets: a line can be
// fine in the wide left corner and too long for the narrow right one.
func TestSanitizeRejectsPerCornerLength(t *testing.T) {
	right := BudgetFor(SideRight)
	long := strings.Repeat("a", right.HardMaxRunes()+1)

	if _, err := Sanitize(Config{Right: Corner{Messages: []Message{{Text: long}}}}); err == nil {
		t.Error("expected the right corner to reject a line past its budget")
	}
	// Same string on the left, which has a wider budget, is accepted.
	if _, err := Sanitize(Config{Left: Corner{Messages: []Message{{Text: long}}}}); err != nil {
		t.Errorf("left corner rejected a line inside its budget: %v", err)
	}
}

func TestSanitizeErrorNamesTheOffendingLine(t *testing.T) {
	long := strings.Repeat("b", BudgetFor(SideRight).HardMaxRunes()+1)
	_, err := Sanitize(Config{Right: Corner{
		Messages: []Message{{Text: "fine"}, {Text: long}},
	}})

	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("error = %v, want a *ValidationError", err)
	}
	if verr.Index != 1 || verr.Side != SideRight || verr.Pool != "messages" {
		t.Errorf("error located %s %s[%d], want right messages[1]", verr.Side, verr.Pool, verr.Index)
	}
}

func TestSanitizeRejectsExcessiveWeight(t *testing.T) {
	_, err := Sanitize(Config{Left: Corner{
		Messages: []Message{{Text: "hog", Weight: MaxWeight + 1}},
	}})
	if err == nil {
		t.Error("expected a weight above MaxWeight to be rejected")
	}
}

func TestSanitizeRejectsOversizedPool(t *testing.T) {
	msgs := make([]Message, MaxMessagesPerPool+1)
	for i := range msgs {
		msgs[i] = Message{Text: "line"}
	}
	if _, err := Sanitize(Config{Left: Corner{Messages: msgs}}); err == nil {
		t.Error("expected a pool above MaxMessagesPerPool to be rejected")
	}
}

// Saving an empty pool is a legitimate "show nothing in this corner" edit, and
// must not be mistaken for a validation failure.
func TestSanitizeAcceptsEmptyConfig(t *testing.T) {
	got, err := Sanitize(Config{})
	if err != nil {
		t.Fatalf("Sanitize(empty): %v", err)
	}
	if len(got.Left.Messages) != 0 || len(got.Right.Messages) != 0 {
		t.Errorf("expected empty pools, got %+v", got)
	}
}

// The shipped defaults must survive their own validation — otherwise the console
// couldn't save a platform it had only prefilled.
func TestSanitizeAcceptsShippedDefaults(t *testing.T) {
	for _, platform := range []string{PlatformTwitch, PlatformYouTube, PlatformTikTok, PlatformInstagram} {
		if _, err := Sanitize(DefaultConfigFor(platform)); err != nil {
			t.Errorf("%s defaults failed validation: %v", platform, err)
		}
	}
}
