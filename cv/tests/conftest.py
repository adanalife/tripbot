"""Shared fixtures + availability gates.

The CV tests fall in three tiers by what they need:
  - pure unit: always run (no fixtures).
  - clip-backed: need the real LFS test clip (skipped in CI, which doesn't
    pull LFS — the MP4 is a pointer file there).
  - model/DB-backed: need the 1.7 GB CLIP checkpoint and a live pgvector DB;
    opt in with DASHCAM_CV_RUN_MODEL_TESTS=1 so CI never downloads the model.
"""

from __future__ import annotations

import os
from pathlib import Path

import pytest

# Checked-in test clip: assets/video/2018_1207_001435_018_opt.MP4 (video_id 4373,
# San Francisco / Golden Gate). LFS-tracked, so in CI it's a small pointer file.
REPO_ROOT = Path(__file__).resolve().parents[2]
TEST_CLIP = REPO_ROOT / "assets" / "video" / "2018_1207_001435_018_opt.MP4"
TEST_CLIP_SLUG = "2018_1207_001435_018_opt"


def clip_available() -> bool:
    return TEST_CLIP.exists() and TEST_CLIP.stat().st_size > 1_000_000


def model_tests_enabled() -> bool:
    return os.environ.get("DASHCAM_CV_RUN_MODEL_TESTS") == "1"


@pytest.fixture
def test_clip() -> Path:
    if not clip_available():
        pytest.skip("test clip unavailable (LFS not pulled?)")
    return TEST_CLIP


@pytest.fixture(scope="session")
def embedder():
    if not model_tests_enabled():
        pytest.skip("set DASHCAM_CV_RUN_MODEL_TESTS=1 to run model-backed tests")
    from dashcam_cv.embed import Embedder

    return Embedder()


@pytest.fixture
def db_conn():
    if not model_tests_enabled():
        pytest.skip("set DASHCAM_CV_RUN_MODEL_TESTS=1 to run DB-backed tests")
    from dashcam_cv import db

    try:
        conn = db.connect()
    except Exception as e:  # noqa: BLE001
        pytest.skip(f"no pgvector DB reachable: {e}")
    yield conn
    conn.close()
