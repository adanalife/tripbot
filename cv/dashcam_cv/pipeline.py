"""embed_video(slug) — the atomic, idempotent unit.

decode -> sample -> CLIP-embed -> discard the frame -> upsert vectors. The
one-off batch loops this over every slug; the future video-pipeline ingest will
call the same function per new clip. ON CONFLICT DO NOTHING makes re-runs (and
overlap between batch and pipeline) safe no-ops.

This is the seam that makes the one-off -> pipeline transition free, so it lives
in its own module rather than buried in the CLI.
"""

from __future__ import annotations

from dataclasses import dataclass

import psycopg

from .corpus import VideoRef
from .embed import Embedder
from .frames import DEFAULT_INTERVAL_SEC, sample_frames

_BATCH = 32

_INSERT_SQL = """
INSERT INTO frame_embeddings (video_id, ts_sec, embedding, model)
VALUES (%s, %s, %s, %s)
ON CONFLICT (video_id, ts_sec, model) DO NOTHING
"""


@dataclass
class EmbedResult:
    """Per-video embedding tally: frames seen and vectors inserted."""

    slug: str
    frames: int
    inserted: int


def embed_video(
    conn: psycopg.Connection,
    embedder: Embedder,
    video: VideoRef,
    interval_sec: float = DEFAULT_INTERVAL_SEC,
    apply: bool = False,
) -> EmbedResult:
    """Embed every sampled frame of one video and upsert the vectors.

    Dry-run (apply=False) decodes + embeds + counts but writes nothing — same
    --apply discipline as cmd/backfill-miles.
    """
    embedder.check_dim()
    model_id = embedder.model_id

    frames_seen = 0
    inserted = 0
    pending_ts: list[float] = []
    pending_imgs: list = []

    def flush() -> None:
        nonlocal inserted, pending_ts, pending_imgs
        if not pending_imgs:
            return
        vecs = embedder.embed_images(pending_imgs)
        if apply:
            with conn.cursor() as cur:
                cur.executemany(
                    _INSERT_SQL,
                    [
                        (video.video_id, ts, vec, model_id)
                        for ts, vec in zip(pending_ts, vecs, strict=True)
                    ],
                )
                inserted += cur.rowcount if cur.rowcount and cur.rowcount > 0 else 0
        pending_ts = []
        pending_imgs = []

    for ts, img in sample_frames(video.path, interval_sec):
        frames_seen += 1
        pending_ts.append(ts)
        pending_imgs.append(img)
        if len(pending_imgs) >= _BATCH:
            flush()
    flush()

    # Commit once per video, not per batch: a video is all-or-nothing. If the
    # pod is interrupted mid-video (it runs at low priority and prod can preempt
    # it), the uncommitted rows roll back, so find_unembedded re-selects the
    # video next run and it gets fully embedded — rather than being left partial
    # and skipped forever. ON CONFLICT DO NOTHING keeps the re-run idempotent.
    if apply:
        conn.commit()

    return EmbedResult(slug=video.slug, frames=frames_seen, inserted=inserted)
