"""Pure unit tests for the natural-language query parser — no model or DB."""

# pylint: disable=missing-function-docstring
from __future__ import annotations

import pytest

from dashcam_cv.query import parse_query


def test_construction_in_california():
    pq = parse_query("construction in California")
    assert pq.visual == "construction"
    assert pq.states == ["California"]
    assert pq.months == []


def test_sunset_in_may():
    pq = parse_query("sunset in May")
    assert pq.visual == "sunset"
    assert pq.months == [5]
    assert pq.states == []


def test_new_england_water_no_preposition():
    # No "in"; region precedes the visual term; region expands to its states.
    pq = parse_query("New England water")
    assert pq.visual == "water"
    assert set(pq.states) == {
        "Maine", "New Hampshire", "Vermont", "Massachusetts",
        "Rhode Island", "Connecticut",
    }


def test_region_alias_and_season():
    pq = parse_query("rain in the PNW in winter")
    assert pq.visual == "rain"
    assert set(pq.states) == {"Washington", "Oregon", "Idaho"}
    assert pq.months == [12, 1, 2]


def test_longest_phrase_wins():
    # "New York" must beat "York"-less "new"; "West Virginia" beats "Virginia".
    assert parse_query("a bridge in New York").states == ["New York"]
    pq = parse_query("a tunnel in West Virginia")
    assert pq.states == ["West Virginia"]
    assert pq.visual == "a tunnel"


def test_uppercase_abbrev_matches_but_lowercase_preposition_does_not():
    # "in" must stay a preposition (not Indiana); uppercase "NV" is Nevada.
    assert parse_query("desert in NV").states == ["Nevada"]
    pq = parse_query("a road in the desert")
    assert pq.states == []  # the lowercase "in" is not Indiana
    assert pq.visual == "a road desert"


def test_hyphen_normalized():
    assert set(parse_query("a city in mid-atlantic").states) == {
        "New York", "New Jersey", "Pennsylvania", "Delaware",
        "Maryland", "Virginia", "West Virginia",
    }


def test_dedupes_states():
    # Region + an explicit member state shouldn't double-list it.
    pq = parse_query("water in New England and Maine")
    assert pq.states.count("Maine") == 1


def test_no_facets_passthrough():
    pq = parse_query("a semi truck at a truck stop")
    assert pq.visual == "a semi truck a truck stop"  # "at" filler dropped
    assert not pq.has_filters


@pytest.mark.parametrize(
    "q,visual",
    [
        ("snow in Colorado", "snow"),
        ("fog over the ocean in Oregon", "fog over ocean"),
        ("a covered bridge in Vermont in October", "a covered bridge"),
    ],
)
def test_visual_residual(q, visual):
    assert parse_query(q).visual == visual
