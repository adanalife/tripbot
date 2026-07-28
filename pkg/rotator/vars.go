package rotator

import (
	"regexp"
	"strings"
)

// Variable is one $token a rotator line can embed, resolved against the
// currently-playing clip when the line renders. Declaring the set here — rather
// than in the renderer and again in the console's insert menu — is what keeps the
// editor's palette, its validation, and what onscreens-server can actually
// substitute from drifting apart.
//
// Example is a plausibly *long* value, not a short one: it stands in for the
// token when copy is measured against a corner's width, so an optimistic example
// would let a line through that wraps on air.
type Variable struct {
	Name        string `json:"name"` // no leading "$"
	Description string `json:"description"`
	Example     string `json:"example"`
}

// Token is the variable as it's written in copy.
func (v Variable) Token() string { return "$" + v.Name }

// variables is the substitutable set, in the order the console lists them.
// Everything here comes off the location feed tripbot publishes for the playing
// clip (pkg/locationfeed → the location.update subject), so adding one means
// adding a field there too — a name with nothing feeding it would validate at
// save time and then never render.
var variables = []Variable{
	{
		Name:        "location",
		Description: "where the clip was filmed — city and state, or just the state before the city resolves",
		Example:     "Grand Junction, Colorado",
	},
	{
		Name:        "state",
		Description: "the state the clip was filmed in, on its own",
		Example:     "West Virginia",
	},
	{
		Name:        "date",
		Description: "the day the clip was filmed",
		Example:     "Wednesday September 26, 2018",
	},
	{
		Name:        "weather",
		Description: "conditions where and when the clip was filmed",
		Example:     "Thunderstorm with hail, 102°F",
	},
	{
		Name:        "sunset",
		Description: "sunset that day at that spot, in local time",
		Example:     "10:42 PM",
	},
}

// Variables returns the substitutable set, for handing to the console alongside
// the copy so its insert menu is generated rather than hardcoded.
func Variables() []Variable { return append([]Variable(nil), variables...) }

// varTokenRE matches a "$name" token. Names are lowercase and must start with a
// letter, so prices ("$5 a month") and a bare "$" aren't mistaken for variables.
var varTokenRE = regexp.MustCompile(`\$([a-z][a-z0-9_]*)`)

// VariablesIn returns the variable names referenced in text, in order, without
// their leading "$". Duplicates are kept — a caller reporting unknown names wants
// each mention.
func VariablesIn(text string) []string {
	matches := varTokenRE.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return nil
	}
	names := make([]string, 0, len(matches))
	for _, m := range matches {
		names = append(names, m[1])
	}
	return names
}

// UnknownVariablesIn returns the names in text that aren't declared above — the
// typo case ("$loction"), caught when copy is saved rather than by rendering the
// literal token on stream.
func UnknownVariablesIn(text string) []string {
	var out []string
	for _, name := range VariablesIn(text) {
		if _, ok := variableByName(name); !ok {
			out = append(out, name)
		}
	}
	return out
}

func variableByName(name string) (Variable, bool) {
	for _, v := range variables {
		if v.Name == name {
			return v, true
		}
	}
	return Variable{}, false
}

// ExpandExamples substitutes each variable's Example, giving the representative
// text a line renders as. Used to measure authored copy against a corner's width,
// where the tokens are the wrong thing to measure.
func ExpandExamples(text string) string {
	return varTokenRE.ReplaceAllStringFunc(text, func(token string) string {
		if v, ok := variableByName(strings.TrimPrefix(token, "$")); ok {
			return v.Example
		}
		return token
	})
}

// Vars holds the live values a render substitutes, keyed by variable name. A name
// that's missing or empty is unknown *right now* (no geocode yet, an archive
// lookup that failed) — distinct from a name that isn't declared at all.
type Vars map[string]string

// Resolvable reports whether every variable text references currently has a
// value, so the line can render fully. False for a line whose data isn't in yet;
// Pick then passes it over rather than putting a bare "$weather" on screen.
func (v Vars) Resolvable(text string) bool {
	for _, name := range VariablesIn(text) {
		if v[name] == "" {
			return false
		}
	}
	return true
}

// Expand substitutes the live values into text. Only call it on text Resolvable
// approves — an unresolved token is left as written, which is the state Pick's
// eligibility check exists to keep off the stream.
func (v Vars) Expand(text string) string {
	if len(v) == 0 {
		return text
	}
	return varTokenRE.ReplaceAllStringFunc(text, func(token string) string {
		if val := v[strings.TrimPrefix(token, "$")]; val != "" {
			return val
		}
		return token
	})
}
