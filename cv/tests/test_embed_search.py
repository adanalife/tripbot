"""End-to-end round trip: embed the test clip, then search for it.

Gated behind DASHCAM_CV_RUN_MODEL_TESTS=1 (needs the CLIP checkpoint + a live
pgvector DB with the frame_embeddings schema migrated).
"""

from __future__ import annotations

import pytest
from conftest import TEST_CLIP, TEST_CLIP_SLUG

from dashcam_cv.corpus import VideoRef
from dashcam_cv.pipeline import embed_video
from dashcam_cv.search import search


@pytest.fixture
def video_id(db_conn) -> int:
    with db_conn.cursor() as cur:
        cur.execute("SELECT id FROM videos WHERE slug = %s", (TEST_CLIP_SLUG,))
        row = cur.fetchone()
    if row is None:
        pytest.skip(f"{TEST_CLIP_SLUG} not seeded in videos")
    return row[0]


def test_embed_then_find(db_conn, embedder, video_id):
    ref = VideoRef(video_id=video_id, slug=TEST_CLIP_SLUG, path=TEST_CLIP)

    # Idempotent clean slate for this video+model so the test is repeatable.
    with db_conn.cursor() as cur:
        cur.execute(
            "DELETE FROM frame_embeddings WHERE video_id = %s AND model = %s",
            (video_id, embedder.model_id),
        )
    db_conn.commit()

    result = embed_video(db_conn, embedder, ref, interval_sec=10.0, apply=True)
    assert result.frames > 0
    assert result.inserted == result.frames

    # Re-running is a no-op (ON CONFLICT DO NOTHING).
    again = embed_video(db_conn, embedder, ref, interval_sec=10.0, apply=True)
    assert again.inserted == 0

    hits = search(db_conn, embedder, "city street", k=5)
    assert hits, "expected search results"
    assert all(0.0 <= h.similarity <= 1.0 for h in hits)
    # The only embedded video is the test clip, so it must dominate the results.
    assert hits[0].slug == TEST_CLIP_SLUG
