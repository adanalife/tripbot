"""Natural-language query parsing for `find`.

Turns a free-text query the audience actually types — "construction in
California", "sunset in May", "New England water" — into a visual residual plus
structured place/time filters. The visual residual goes to the embedding; the
states/months become a SQL filter on the videos join (see search.py). This is
why `!find` can take human language with no flags.

Deterministic gazetteer: a bounded, closed vocabulary (50 states + USPS
abbreviations, a handful of named regions, months, seasons) that covers the
overwhelming majority of audience phrasing instantly and with no dependency. An
LLM parser (Claude Haiku + structured output — the planned "LLM tagging via
NATS" service) is the documented upgrade path for the long tail: typos, fuzzy
time ("around Christmas"), cities/landmarks ("near Moab"). When that lands, call
it only as a fallback when this parser extracts nothing.

Deliberate v1 choices, documented so they're decisions not accidents:
  - Region boundaries are opinionated and fuzzy (is Texas "the South" or "the
    Southwest"?). The maps below are reasonable, not canonical.
  - Two-letter state abbreviations match ONLY when uppercase in the original
    ("CA", "NV") — lowercase "in"/"or"/"me" are prepositions and pronouns far
    more often than Indiana/Oregon/Maine, so matching them would wreck the
    common case ("construction in california").
  - "May"/"March" are also English words; we still read them as months, since a
    visual query rarely needs them. Re-evaluate if it bites.
  - Time-of-day ("sunset", "at night") stays purely visual — local time needs
    each clip's timezone from its lat/lng, which is a later enhancement.
"""

from __future__ import annotations

import re
from dataclasses import dataclass, field

STATES = [
    "Alabama", "Alaska", "Arizona", "Arkansas", "California", "Colorado",
    "Connecticut", "Delaware", "Florida", "Georgia", "Hawaii", "Idaho",
    "Illinois", "Indiana", "Iowa", "Kansas", "Kentucky", "Louisiana", "Maine",
    "Maryland", "Massachusetts", "Michigan", "Minnesota", "Mississippi",
    "Missouri", "Montana", "Nebraska", "Nevada", "New Hampshire", "New Jersey",
    "New Mexico", "New York", "North Carolina", "North Dakota", "Ohio",
    "Oklahoma", "Oregon", "Pennsylvania", "Rhode Island", "South Carolina",
    "South Dakota", "Tennessee", "Texas", "Utah", "Vermont", "Virginia",
    "Washington", "West Virginia", "Wisconsin", "Wyoming",
]  # fmt: skip

ABBREV = {
    "AL": "Alabama", "AK": "Alaska", "AZ": "Arizona", "AR": "Arkansas",
    "CA": "California", "CO": "Colorado", "CT": "Connecticut", "DE": "Delaware",
    "FL": "Florida", "GA": "Georgia", "HI": "Hawaii", "ID": "Idaho",
    "IL": "Illinois", "IN": "Indiana", "IA": "Iowa", "KS": "Kansas",
    "KY": "Kentucky", "LA": "Louisiana", "ME": "Maine", "MD": "Maryland",
    "MA": "Massachusetts", "MI": "Michigan", "MN": "Minnesota",
    "MS": "Mississippi", "MO": "Missouri", "MT": "Montana", "NE": "Nebraska",
    "NV": "Nevada", "NH": "New Hampshire", "NJ": "New Jersey", "NM": "New Mexico",
    "NY": "New York", "NC": "North Carolina", "ND": "North Dakota", "OH": "Ohio",
    "OK": "Oklahoma", "OR": "Oregon", "PA": "Pennsylvania", "RI": "Rhode Island",
    "SC": "South Carolina", "SD": "South Dakota", "TN": "Tennessee",
    "TX": "Texas", "UT": "Utah", "VT": "Vermont", "VA": "Virginia",
    "WA": "Washington", "WV": "West Virginia", "WI": "Wisconsin", "WY": "Wyoming",
}  # fmt: skip

# Opinionated, fuzzy. Hyphen/space variants are normalized at match time.
REGIONS = {
    "new england": ["Maine", "New Hampshire", "Vermont", "Massachusetts",
                    "Rhode Island", "Connecticut"],
    "mid atlantic": ["New York", "New Jersey", "Pennsylvania", "Delaware",
                     "Maryland", "Virginia", "West Virginia"],
    "the south": ["Virginia", "North Carolina", "South Carolina", "Georgia",
                  "Florida", "Alabama", "Mississippi", "Tennessee", "Kentucky",
                  "Louisiana", "Arkansas"],
    "deep south": ["Georgia", "Alabama", "Mississippi", "Louisiana",
                   "South Carolina"],
    "midwest": ["Ohio", "Indiana", "Illinois", "Michigan", "Wisconsin",
                "Minnesota", "Iowa", "Missouri", "Kansas", "Nebraska",
                "North Dakota", "South Dakota"],
    "southwest": ["Arizona", "New Mexico", "Nevada", "Utah"],
    "pacific northwest": ["Washington", "Oregon", "Idaho"],
    "west coast": ["California", "Oregon", "Washington"],
    "east coast": ["Maine", "New Hampshire", "Massachusetts", "Rhode Island",
                   "Connecticut", "New York", "New Jersey", "Delaware",
                   "Maryland", "Virginia", "North Carolina", "South Carolina",
                   "Georgia", "Florida"],
    "mountain west": ["Montana", "Idaho", "Wyoming", "Colorado", "Utah",
                      "Nevada"],
    "the rockies": ["Montana", "Idaho", "Wyoming", "Colorado", "Utah"],
    "great plains": ["North Dakota", "South Dakota", "Nebraska", "Kansas",
                     "Oklahoma"],
}
# Common aliases that resolve to a region key above.
REGION_ALIASES = {"pnw": "pacific northwest", "the rockies": "the rockies",
                  "rockies": "the rockies", "the midwest": "midwest",
                  "the southwest": "southwest", "the northeast": "new england"}

MONTHS = {
    "january": 1, "february": 2, "march": 3, "april": 4, "may": 5, "june": 6,
    "july": 7, "august": 8, "september": 9, "october": 10, "november": 11,
    "december": 12, "jan": 1, "feb": 2, "mar": 3, "apr": 4, "jun": 6, "jul": 7,
    "aug": 8, "sep": 9, "sept": 9, "oct": 10, "nov": 11, "dec": 12,
}
SEASONS = {
    "spring": [3, 4, 5], "summer": [6, 7, 8], "fall": [9, 10, 11],
    "autumn": [9, 10, 11], "winter": [12, 1, 2],
}

# Prepositions left dangling once a facet is removed ("... in <state>"). Dropped
# from the visual residual; none of our visual concepts need them as content.
_FILLER = {"in", "from", "during", "around", "near", "at", "the"}


@dataclass
class ParsedQuery:
    """A free-text query split into a visual residual and structured filters."""

    visual: str
    states: list[str] = field(default_factory=list)
    months: list[int] = field(default_factory=list)
    # Human-readable description of what was pulled out, for transparency.
    notes: list[str] = field(default_factory=list)
    raw: str = ""

    @property
    def has_filters(self) -> bool:
        """True if any place/time facet was extracted."""
        return bool(self.states or self.months)


def _phrase_table() -> dict[str, tuple[str, object]]:
    """Map a lowercased phrase -> (kind, payload) for the case-insensitive pass."""
    table: dict[str, tuple[str, object]] = {}
    for name in STATES:
        table[name.lower()] = ("state", [name])
    for phrase, states in REGIONS.items():
        table[phrase] = ("region", (phrase, states))
    for alias, key in REGION_ALIASES.items():
        table[alias] = ("region", (alias, REGIONS[key]))
    for word, num in MONTHS.items():
        table[word] = ("month", [num])
    for word, nums in SEASONS.items():
        table[word] = ("season", (word, nums))
    return table


_TABLE = _phrase_table()
# Longest phrases first so "new york" wins over "new", "west virginia" over
# "virginia"; non-overlapping finditer then consumes the longer span.
_PHRASE_RE = re.compile(
    r"\b(" + "|".join(re.escape(p) for p in sorted(_TABLE, key=len, reverse=True)) + r")\b"
)
_ABBREV_RE = re.compile(r"\b[A-Z]{2}\b")  # uppercase-only, matched on the original


def parse_query(text: str) -> ParsedQuery:
    """Parse free text into a visual residual + place/time filters.

    Hyphens are treated as spaces ("mid-atlantic" == "mid atlantic"). Matched
    facet spans are blanked out of the residual; leftover prepositions are
    dropped. States/months are de-duplicated while preserving first-seen order.
    """
    raw = text
    norm = text.replace("-", " ")
    lowered = norm.lower()
    spans: list[tuple[int, int]] = []
    states: list[str] = []
    months: list[int] = []
    notes: list[str] = []

    def add_states(names: list[str]) -> None:
        for n in names:
            if n not in states:
                states.append(n)

    def add_months(nums: list[int]) -> None:
        for m in nums:
            if m not in months:
                months.append(m)

    for m in _PHRASE_RE.finditer(lowered):
        kind, payload = _TABLE[m.group(1)]
        spans.append((m.start(), m.end()))
        if kind == "state":
            add_states(payload)
            notes.append(f"state={payload[0]}")
        elif kind == "region":
            phrase, region_states = payload
            add_states(region_states)
            notes.append(f"region={phrase.title()} ({len(region_states)} states)")
        elif kind == "month":
            add_months(payload)
            notes.append(_month_name(payload[0]))
        elif kind == "season":
            season, nums = payload
            add_months(nums)
            notes.append(f"{season} (months {', '.join(map(str, nums))})")

    for m in _ABBREV_RE.finditer(norm):  # original case for abbreviations
        code = m.group(0)
        if code in ABBREV and not _overlaps(spans, m.start(), m.end()):
            name = ABBREV[code]
            spans.append((m.start(), m.end()))
            if name not in states:
                states.append(name)
                notes.append(f"state={name}")

    visual = _residual(norm, spans)
    return ParsedQuery(visual=visual, states=states, months=months, notes=notes, raw=raw)


def _overlaps(spans: list[tuple[int, int]], start: int, end: int) -> bool:
    """True if [start, end) intersects any already-claimed span."""
    return any(start < e and s < end for s, e in spans)


def _residual(text: str, spans: list[tuple[int, int]]) -> str:
    """Blank the matched spans, drop filler words, collapse whitespace."""
    chars = list(text)
    for s, e in spans:
        for i in range(s, e):
            chars[i] = " "
    leftover = "".join(chars)
    words = [w for w in leftover.split() if w.lower() not in _FILLER]
    return " ".join(words)


def _month_name(num: int) -> str:
    """Month number -> full English name (for the notes display)."""
    return [
        "January", "February", "March", "April", "May", "June", "July",
        "August", "September", "October", "November", "December",
    ][num - 1]
