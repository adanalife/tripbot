"""Pure unit tests — no model, DB, or clip required."""

from __future__ import annotations

import pytest

from dashcam_cv.cli import _format_ts, main
from dashcam_cv.db import EMBED_DIM, dsn
from dashcam_cv.search import SearchHit


def test_embed_dim_matches_model():
    # The production model (SigLIP2 so400m NaFlex) and the vector(1152) column.
    assert EMBED_DIM == 1152


def test_dsn_from_env(monkeypatch):
    monkeypatch.setenv("DATABASE_USER", "u")
    monkeypatch.setenv("DATABASE_PASS", "p")
    monkeypatch.setenv("DATABASE_HOST", "h")
    monkeypatch.setenv("DATABASE_PORT", "5499")
    monkeypatch.setenv("DATABASE_DB", "d")
    assert dsn() == "postgresql://u:p@h:5499/d?sslmode=disable"


@pytest.mark.parametrize(
    "secs,expected",
    [(0, "0:00"), (5, "0:05"), (65, "1:05"), (619, "10:19"), (3661, "1:01:01")],
)
def test_format_ts(secs, expected):
    assert _format_ts(secs) == expected


def test_similarity_from_distance():
    hit = SearchHit(video_id=1, slug="s", ts_sec=1.0, distance=0.25, state="California")
    assert hit.similarity == pytest.approx(0.75)


def test_cli_requires_subcommand():
    with pytest.raises(SystemExit) as exc:
        main([])
    assert exc.value.code == 2
