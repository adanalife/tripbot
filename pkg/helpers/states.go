package helpers

import (
	"regexp"
	"strings"
)

// characters that can't appear in a state or territory name
var nonStateNameChars = regexp.MustCompile(`[^a-zA-Z\s]+`)

// NormalizeStateInput cleans up a user-supplied state name so it can be looked
// up: characters a state name can't contain (digits, punctuation, emoji) are
// dropped, runs of whitespace collapse to a single space, and the ends are
// trimmed. Interior spaces survive, so multi-word names like "new york" stay
// intact. Case is left alone — TitlecaseState resolves that.
func NormalizeStateInput(input string) string {
	return strings.Join(strings.Fields(nonStateNameChars.ReplaceAllString(input, "")), " ")
}

func StateAbbrevToState(abbrev string) string {
	val, ok := stateAbbrevs[strings.ToUpper(abbrev)]
	if !ok {
		return ""
	}
	return val
}

func StateToStateAbbrev(state string) string {
	// Case-insensitive lookup so names like "district of columbia" still
	// resolve (title-casing the input would mis-capitalise the internal
	// "of"/"and" words in multi-word names).
	want := strings.ToLower(state)
	for abbrev, name := range stateAbbrevs {
		if strings.ToLower(name) == want {
			return abbrev
		}
	}
	return ""
}

// StateNames returns the full state/territory names from the abbreviation
// table. Order is unstable (map iteration); callers must not rely on it.
func StateNames() []string {
	names := make([]string, 0, len(stateAbbrevs))
	for _, name := range stateAbbrevs {
		names = append(names, name)
	}
	return names
}

// TitlecaseState renders a US state or territory in the canonical display form
// used throughout the app (and stored in the videos table). It accepts either a
// two-letter abbreviation ("ca", "DC") or a full name ("california"), ignoring
// case and surrounding whitespace, and returns the table's spelling — so "DC"
// becomes "District of Columbia" rather than "District Of Columbia".
//
// Input that names no known state is echoed back title-cased instead of being
// swallowed, so callers that render the result (chat replies, onscreens) show
// what was asked for rather than an empty string.
func TitlecaseState(state string) string {
	state = strings.TrimSpace(state)
	if name := StateAbbrevToState(state); name != "" {
		return name
	}
	if abbrev := StateToStateAbbrev(state); abbrev != "" {
		return stateAbbrevs[abbrev]
	}
	return strings.Title(strings.ToLower(state))
}

// A handy map of US state codes to full names
var stateAbbrevs = map[string]string{
	"AL": "Alabama",
	"AK": "Alaska",
	"AZ": "Arizona",
	"AR": "Arkansas",
	"CA": "California",
	"CO": "Colorado",
	"CT": "Connecticut",
	"DE": "Delaware",
	"FL": "Florida",
	"GA": "Georgia",
	"HI": "Hawaii",
	"ID": "Idaho",
	"IL": "Illinois",
	"IN": "Indiana",
	"IA": "Iowa",
	"KS": "Kansas",
	"KY": "Kentucky",
	"LA": "Louisiana",
	"ME": "Maine",
	"MD": "Maryland",
	"MA": "Massachusetts",
	"MI": "Michigan",
	"MN": "Minnesota",
	"MS": "Mississippi",
	"MO": "Missouri",
	"MT": "Montana",
	"NE": "Nebraska",
	"NV": "Nevada",
	"NH": "New Hampshire",
	"NJ": "New Jersey",
	"NM": "New Mexico",
	"NY": "New York",
	"NC": "North Carolina",
	"ND": "North Dakota",
	"OH": "Ohio",
	"OK": "Oklahoma",
	"OR": "Oregon",
	"PA": "Pennsylvania",
	"RI": "Rhode Island",
	"SC": "South Carolina",
	"SD": "South Dakota",
	"TN": "Tennessee",
	"TX": "Texas",
	"UT": "Utah",
	"VT": "Vermont",
	"VA": "Virginia",
	"WA": "Washington",
	"WV": "West Virginia",
	"WI": "Wisconsin",
	"WY": "Wyoming",
	// Territories
	"AS": "American Samoa",
	"DC": "District of Columbia",
	"FM": "Federated States of Micronesia",
	"GU": "Guam",
	"MH": "Marshall Islands",
	"MP": "Northern Mariana Islands",
	"PW": "Palau",
	"PR": "Puerto Rico",
	"VI": "Virgin Islands",
	// Armed Forces (AE includes Europe, Africa, Canada, and the Middle East)
	"AA": "Armed Forces Americas",
	"AE": "Armed Forces Europe",
	"AP": "Armed Forces Pacific",
}
