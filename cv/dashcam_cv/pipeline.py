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
from .frames import DEFAULT_INTERVAL_SEC, mean_luminance, sample_frames

_BATCH = 32

# Skip frames darker than this mean luminance (0-255). Tunnel mouths, camera
# startup, and glitches produce near-black frames whose embeddings are noise;
# dropping them before the model call saves compute and keeps junk out of
# search. Set 0 to disable (nothing is < 0).
DEFAULT_BLACK_THRESHOLD = 10.0

_INSERT_SQL = """
INSERT INTO frame_embeddings (video_id, ts_sec, embedding, model)
VALUES (%s, %s, %s, %s)
ON CONFLICT (video_id, ts_sec, model) DO NOTHING
"""


# Skip a sampled frame whose embedding is at least this cosine-similar to the
# last kept frame's. Slow-TV footage lingers on near-identical scenes (stopped
# at a light, long straight road), so consecutive frames are often redundant;
# dropping them shrinks the vector store and keeps search results diverse rather
# than dominated by one near-static stretch. Conservative by default — only very
# near-duplicates go. Set >= 1.0 to disable (no cosine exceeds 1.0).
DEFAULT_DEDUP_THRESHOLD = 0.98


@dataclass
class EmbedResult:
    """Per-video tally: frames sampled, vectors written, dupes/dark skipped."""

    slug: str
    frames: int
    inserted: int
    deduped: int
    dark: int


def embed_video(
    conn: psycopg.Connection,
    embedder: Embedder,
    video: VideoRef,
    interval_sec: float = DEFAULT_INTERVAL_SEC,
    apply: bool = False,
    dedup_threshold: float = DEFAULT_DEDUP_THRESHOLD,
    black_threshold: float = DEFAULT_BLACK_THRESHOLD,
) -> EmbedResult:
    """Embed every sampled frame of one video and upsert the vectors.

    Near-black frames (mean luminance < black_threshold) are dropped before the
    model call; near-duplicate frames (cosine > dedup_threshold vs the last kept
    frame) are dropped after. Dry-run (apply=False) decodes + embeds + counts but
    writes nothing — same --apply discipline as cmd/backfill-miles.
    """
    embedder.check_dim()
    model_id = embedder.model_id

    frames_seen = 0
    inserted = 0
    deduped = 0
    dark = 0
    last_kept = None  # embedding of the last frame we kept (for dedup)
    pending_ts: list[float] = []
    pending_imgs: list = []

    def flush() -> None:
        nonlocal inserted, deduped, last_kept, pending_ts, pending_imgs
        if not pending_imgs:
            return
        vecs = embedder.embed_images(pending_imgs)
        rows = []
        for ts, vec in zip(pending_ts, vecs, strict=True):
            # vecs are L2-normalized, so the dot product is cosine similarity.
            if last_kept is not None and float(last_kept @ vec) > dedup_threshold:
                deduped += 1
                continue
            rows.append((video.video_id, ts, vec, model_id))
            last_kept = vec
        if apply and rows:
            with conn.cursor() as cur:
                cur.executemany(_INSERT_SQL, rows)
                inserted += cur.rowcount if cur.rowcount and cur.rowcount > 0 else 0
        pending_ts = []
        pending_imgs = []

    for ts, img in sample_frames(video.path, interval_sec):
        frames_seen += 1
        # Drop near-black frames before embedding (saves the model call).
        if black_threshold > 0 and mean_luminance(img) < black_threshold:
            dark += 1
            continue
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

    return EmbedResult(
        slug=video.slug, frames=frames_seen, inserted=inserted, deduped=deduped, dark=dark
    )
