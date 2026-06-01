"""Locate corpus files and join them to their `videos` rows by slug.

slug == filename without extension (matches pkg/video's slug()/File()):
  2018_1207_001435_018_opt.MP4  ->  slug 2018_1207_001435_018_opt
"""

from __future__ import annotations

import os
from dataclasses import dataclass
from pathlib import Path

import psycopg

# Read-only corpus mount in every cluster env; override locally (e.g. point at
# the checked-in assets/video clip) with DASHCAM_CV_CORPUS_DIR.
DEFAULT_CORPUS_DIR = "/opt/data/Dashcam/_all"


@dataclass(frozen=True)
class VideoRef:
    """A corpus file matched to its videos row: (id, slug, on-disk path)."""

    video_id: int
    slug: str
    path: Path


def corpus_dir() -> Path:
    """Corpus root — DASHCAM_CV_CORPUS_DIR, or the cluster mount by default."""
    return Path(os.environ.get("DASHCAM_CV_CORPUS_DIR", DEFAULT_CORPUS_DIR))


def _slug_to_id(conn: psycopg.Connection) -> dict[str, int]:
    """Map every videos.slug to its id."""
    with conn.cursor() as cur:
        cur.execute("SELECT slug, id FROM videos")
        return dict(cur.fetchall())


def find_videos(
    conn: psycopg.Connection,
    directory: Path | None = None,
    slugs: list[str] | None = None,
    limit: int | None = None,
) -> tuple[list[VideoRef], list[str]]:
    """Return (matched VideoRefs, orphan slugs).

    Orphans are *.MP4 files on disk with no matching `videos` row — surfaced so
    the caller can report them rather than silently skipping.
    """
    directory = directory or corpus_dir()
    slug_to_id = _slug_to_id(conn)
    wanted = set(slugs) if slugs else None

    matched: list[VideoRef] = []
    orphans: list[str] = []
    for path in sorted(directory.glob("*.MP4")):
        slug = path.stem
        if wanted is not None and slug not in wanted:
            continue
        vid = slug_to_id.get(slug)
        if vid is None:
            orphans.append(slug)
            continue
        matched.append(VideoRef(video_id=vid, slug=slug, path=path))
        if limit is not None and len(matched) >= limit:
            break
    return matched, orphans


def find_unembedded(
    conn: psycopg.Connection,
    model_id: str,
    directory: Path | None = None,
    n: int = 5,
) -> list[VideoRef]:
    """Return up to n *random* videos with no embeddings yet for `model_id`.

    The unit the incremental mini-PC job processes: pick random un-treated videos,
    embed them, repeat. Idempotent + self-converging — re-runs never redo a video
    that already has rows for this model, so running the job on a schedule
    gradually fills the corpus and sprinkles variety into !find. Flagged
    placeholder rows and files missing on disk are skipped.
    """
    directory = directory or corpus_dir()
    with conn.cursor() as cur:
        cur.execute(
            """
            SELECT v.id, v.slug
            FROM videos v
            WHERE v.flagged = false
              AND NOT EXISTS (
                SELECT 1 FROM frame_embeddings fe
                WHERE fe.video_id = v.id AND fe.model = %s
              )
            ORDER BY random()
            """,
            (model_id,),
        )
        candidates = cur.fetchall()

    refs: list[VideoRef] = []
    for vid, slug in candidates:
        path = directory / f"{slug}.MP4"
        if path.exists():
            refs.append(VideoRef(video_id=vid, slug=slug, path=path))
            if len(refs) >= n:
                break
    return refs
