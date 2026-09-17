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
	// strings.Title is deprecated in favour of x/text's cases.Title, which is
	// the wrong trade here: this path echoes back whatever a viewer typed, and
	// cases.Title treats a digit as a word boundary ("3rd street" becomes "3Rd
	// Street"). strings.Title's word rule is the one that reads correctly for
	// arbitrary input, and Go 1 compatibility keeps it from being removed.
	//nolint:staticcheck // SA1019: see above
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

// StateCentroid returns the representative point for a US state — the
// coordinate !guess measures a wrong guess's distance from. It accepts a
// two-letter abbreviation or a full name, ignoring case. Territories aren't in
// the table (no footage there), so they and unknown names report ok=false.
//
// Values are the Google canonical states_csv dataset
// (developers.google.com/public-data/docs/canonical/states_csv). Geographic
// centers, not population centers — the van is on highways, not in cities, and
// warmer/colder only needs the sign of a distance delta to be right.
func StateCentroid(state string) (lat, lng float64, ok bool) {
	abbrev := strings.ToUpper(state)
	if _, isAbbrev := stateAbbrevs[abbrev]; !isAbbrev {
		abbrev = StateToStateAbbrev(state)
	}
	c, ok := stateCentroids[abbrev]
	return c[0], c[1], ok
}

// stateCentroids maps a state abbreviation to its {lat, lng} centroid.
var stateCentroids = map[string][2]float64{
	"AL": {32.806671, -86.791130},
	"AK": {61.370716, -152.404419},
	"AZ": {33.729759, -111.431221},
	"AR": {34.969704, -92.373123},
	"CA": {36.116203, -119.681564},
	"CO": {39.059811, -105.311104},
	"CT": {41.597782, -72.755371},
	"DE": {39.318523, -75.507141},
	"DC": {38.897438, -77.026817},
	"FL": {27.766279, -81.686783},
	"GA": {33.040619, -83.643074},
	"HI": {21.094318, -157.498337},
	"ID": {44.240459, -114.478828},
	"IL": {40.349457, -88.986137},
	"IN": {39.849426, -86.258278},
	"IA": {42.011539, -93.210526},
	"KS": {38.526600, -96.726486},
	"KY": {37.668140, -84.670067},
	"LA": {31.169546, -91.867805},
	"ME": {44.693947, -69.381927},
	"MD": {39.063946, -76.802101},
	"MA": {42.230171, -71.530106},
	"MI": {43.326618, -84.536095},
	"MN": {45.694454, -93.900192},
	"MS": {32.741646, -89.678696},
	"MO": {38.456085, -92.288368},
	"MT": {46.921925, -110.454353},
	"NE": {41.125370, -98.268082},
	"NV": {38.313515, -117.055374},
	"NH": {43.452492, -71.563896},
	"NJ": {40.298904, -74.521011},
	"NM": {34.840515, -106.248482},
	"NY": {42.165726, -74.948051},
	"NC": {35.630066, -79.806419},
	"ND": {47.528912, -99.784012},
	"OH": {40.388783, -82.764915},
	"OK": {35.565342, -96.928917},
	"OR": {44.572021, -122.070938},
	"PA": {40.590752, -77.209755},
	"RI": {41.680893, -71.511780},
	"SC": {33.856892, -80.945007},
	"SD": {44.299782, -99.438828},
	"TN": {35.747845, -86.692345},
	"TX": {31.054487, -97.563461},
	"UT": {40.150032, -111.862434},
	"VT": {44.045876, -72.710686},
	"VA": {37.769337, -78.169968},
	"WA": {47.400902, -121.490494},
	"WV": {38.491226, -80.954453},
	"WI": {44.268543, -89.616508},
	"WY": {42.755966, -107.302490},
}
