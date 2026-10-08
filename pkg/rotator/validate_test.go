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

// A misspelled variable has to be caught here: it would otherwise save cleanly
// and then render "$loction" on a 24/7 stream.
func TestSanitizeRejectsUnknownVariable(t *testing.T) {
	_, err := Sanitize(Config{Left: Corner{
		Messages: []Message{{Text: "fine"}, {Text: "driving through $loction"}},
	}})

	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("error = %v, want a *ValidationError", err)
	}
	if verr.Index != 1 || verr.Pool != "messages" {
		t.Errorf("error located %s[%d], want messages[1]", verr.Pool, verr.Index)
	}
	if !strings.Contains(verr.Msg, "$loction") {
		t.Errorf("message %q should name the offending token", verr.Msg)
	}
	// The message doubles as the list of what *is* available, since the console
	// shows it verbatim.
	for _, v := range Variables() {
		if !strings.Contains(verr.Msg, v.Token()) {
			t.Errorf("message %q should offer %s", verr.Msg, v.Token())
		}
	}
	// The rare line answers to the same check.
	if _, err := Sanitize(Config{RareMessage: "you found $nothing!"}); err == nil {
		t.Error("expected the rare line's unknown variable to be rejected")
	}
}

func TestSanitizeAcceptsDeclaredVariables(t *testing.T) {
	var msgs []Message
	for _, v := range Variables() {
		msgs = append(msgs, Message{Text: v.Token()})
	}
	if _, err := Sanitize(Config{Left: Corner{Messages: msgs}}); err != nil {
		t.Errorf("declared variables rejected: %v", err)
	}
}

// Length is measured against what a line renders as, not what it's written as: a
// short line of tokens can expand past the corner it sits in.
func TestSanitizeMeasuresVariablesExpanded(t *testing.T) {
	// Repeat a token until its expansion clears the right corner's limit while
	// the authored text stays well inside it.
	loc, _ := variableByName("location")
	n := BudgetFor(SideRight).HardMaxRunes()/len(loc.Example) + 2
	text := strings.Repeat(loc.Token()+" ", n)
	if BudgetFor(SideRight).TooLong(text) {
		t.Fatalf("test setup: authored text is already too long (%d runes)", len(text))
	}

	if _, err := Sanitize(Config{Right: Corner{Messages: []Message{{Text: text}}}}); err == nil {
		t.Error("expected a line whose expansion overflows the corner to be rejected")
	}
}
