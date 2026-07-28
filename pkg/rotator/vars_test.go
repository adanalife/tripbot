package rotator

import (
	"strings"
	"testing"
)

// liveVars is a full set of resolved values, as onscreens-server builds from a
// location.update it just received.
var liveVars = Vars{
	"location": "Moab, Utah",
	"state":    "Utah",
	"date":     "Thursday June 14, 2018",
	"weather":  "Clear sky, 88°F",
	"sunset":   "8:42 PM",
}

func TestVariablesIn(t *testing.T) {
	got := VariablesIn("$location — $weather, sunset at $sunset")
	want := []string{"location", "weather", "sunset"}
	if len(got) != len(want) {
		t.Fatalf("VariablesIn = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("VariablesIn[%d] = %q, want %q", i, got[i], want[i])
		}
	}
	if got := VariablesIn("no variables here"); got != nil {
		t.Errorf("expected no variables, got %v", got)
	}
}

// A price is not a variable. Requiring a name to start with a letter is what
// keeps "$5" out of the token grammar, so copy mentioning money doesn't have to
// escape anything and doesn't fail validation.
func TestVariablesInIgnoresPricesAndBareDollars(t *testing.T) {
	for _, text := range []string{"Subs are $5 a month", "100% free — no $", "$ $ $"} {
		if got := VariablesIn(text); got != nil {
			t.Errorf("VariablesIn(%q) = %v, want none", text, got)
		}
		if got := UnknownVariablesIn(text); got != nil {
			t.Errorf("UnknownVariablesIn(%q) = %v, want none", text, got)
		}
	}
}

func TestUnknownVariablesIn(t *testing.T) {
	if got := UnknownVariablesIn("driving through $loction"); len(got) != 1 || got[0] != "loction" {
		t.Errorf("UnknownVariablesIn = %v, want [loction]", got)
	}
	for _, v := range Variables() {
		if got := UnknownVariablesIn("we are in " + v.Token()); got != nil {
			t.Errorf("declared variable %s reported unknown: %v", v.Token(), got)
		}
	}
}

func TestVarsExpand(t *testing.T) {
	got := liveVars.Expand("📍 $location — $weather")
	if want := "📍 Moab, Utah — Clear sky, 88°F"; got != want {
		t.Errorf("Expand = %q, want %q", got, want)
	}
	// Two mentions of the same variable both resolve.
	if got := liveVars.Expand("$state, $state"); got != "Utah, Utah" {
		t.Errorf("Expand of a repeated variable = %q", got)
	}
	// Nothing to substitute leaves the text alone, with or without values.
	if got := liveVars.Expand("plain copy"); got != "plain copy" {
		t.Errorf("Expand of plain copy = %q", got)
	}
	if got := Vars(nil).Expand("$location"); got != "$location" {
		t.Errorf("Expand with no values = %q, want the token untouched", got)
	}
}

func TestVarsResolvable(t *testing.T) {
	if !liveVars.Resolvable("$location at $sunset") {
		t.Error("a line whose variables all have values should be resolvable")
	}
	if !liveVars.Resolvable("no variables") {
		t.Error("a line with no variables is always resolvable")
	}
	if Vars(nil).Resolvable("$location") {
		t.Error("a line should not be resolvable with no values at all")
	}
	// The distinction the rotators depend on: an empty value is "not known yet",
	// which must block the line rather than render as a blank gap.
	partial := Vars{"location": "Moab, Utah", "weather": ""}
	if partial.Resolvable("$location — $weather") {
		t.Error("an empty value should make the line unresolvable")
	}
	if !partial.Resolvable("📍 $location") {
		t.Error("a line using only the populated variable should still resolve")
	}
}

// Pick must never put a bare token on screen: a line whose data hasn't arrived
// is passed over, even when that leaves only the static lines.
func TestPickSkipsUnresolvableLines(t *testing.T) {
	msgs := []Message{
		{Text: "Somewhere in $state"},
		{Text: "static line"},
	}
	for i := 0; i < 2000; i++ {
		if got := Pick(PlatformTwitch, msgs, nil, nil); got != "static line" {
			t.Fatalf("Pick with no values = %q, want only the static line", got)
		}
	}
	// With a value, the variable line renders substituted.
	var sawSubstituted bool
	for i := 0; i < 2000 && !sawSubstituted; i++ {
		got := Pick(PlatformTwitch, msgs, nil, liveVars)
		if strings.Contains(got, "$") {
			t.Fatalf("Pick rendered an unsubstituted token: %q", got)
		}
		if got == "Somewhere in Utah" {
			sawSubstituted = true
		}
	}
	if !sawSubstituted {
		t.Error("expected the variable line to render substituted at least once")
	}
}

// A pool of nothing but unresolvable lines yields an empty corner rather than a
// literal token — the one case where the rotator would rather show nothing.
func TestPickEmptyWhenEveryLineIsUnresolvable(t *testing.T) {
	msgs := []Message{{Text: "$weather right now"}, {Text: "sunset at $sunset"}}
	if got := Pick(PlatformTwitch, msgs, nil, nil); got != "" {
		t.Errorf("Pick = %q, want empty when nothing can resolve", got)
	}
}

// The sibling-exclusion relax must not smuggle an unresolvable line back in: the
// only reason to relax is a duplicate command, which is a lesser evil than a
// literal token.
func TestPickRelaxDoesNotRestoreUnresolvableLines(t *testing.T) {
	msgs := []Message{
		{Text: "Try running `!location` in $state"},
		{Text: "Where are we? (`!location`)"},
	}
	exclude := map[string]bool{"location": true}
	for i := 0; i < 500; i++ {
		if got := Pick(PlatformTwitch, msgs, exclude, nil); strings.Contains(got, "$") {
			t.Fatalf("relaxed pick rendered an unsubstituted token: %q", got)
		}
	}
}

func TestExpandExamplesUsesDeclaredExamples(t *testing.T) {
	got := ExpandExamples("📍 $location")
	loc, ok := variableByName("location")
	if !ok {
		t.Fatal("location variable missing from the declared set")
	}
	if want := "📍 " + loc.Example; got != want {
		t.Errorf("ExpandExamples = %q, want %q", got, want)
	}
	// An undeclared token is left alone — checkVariables is what rejects it, and
	// blanking it here would make the length check optimistic instead.
	if got := ExpandExamples("$loction"); got != "$loction" {
		t.Errorf("ExpandExamples of an unknown token = %q, want it untouched", got)
	}
}

// Every declared variable needs a description and an example — the console
// generates its insert menu from these, and measures copy against the examples.
func TestVariablesAreFullyDeclared(t *testing.T) {
	for _, v := range Variables() {
		if v.Name == "" || v.Description == "" || v.Example == "" {
			t.Errorf("incompletely declared variable: %+v", v)
		}
		if v.Token() != "$"+v.Name {
			t.Errorf("Token() = %q for name %q", v.Token(), v.Name)
		}
		// A name the token grammar can't match would validate and never render.
		if got := VariablesIn(v.Token()); len(got) != 1 || got[0] != v.Name {
			t.Errorf("%s doesn't match the token grammar: %v", v.Token(), got)
		}
	}
}

// Variables() hands out a copy, so a caller (the API DTO round-trip) can't
// scribble on the package's declared set.
func TestVariablesIsolatesCallers(t *testing.T) {
	first := Variables()
	original := first[0].Name
	first[0].Name = "scribbled"
	if got := Variables()[0].Name; got != original {
		t.Errorf("mutating the returned slice changed the declared set: %q", got)
	}
}
