"""Corpus embedding stats — coverage, DB size, and a concept scan.

Powers `dashcam-cv stats`, a check-in view for watching the corpus fill.
Coverage + size are plain SQL (fast, no model). The concept scan embeds a
curated road-trip vocabulary and counts how many frames each word matches —
common concepts (sky, road, trees) float to the top, rare ones (tunnel, snow)
sink — so it needs the model.
"""

from __future__ import annotations

from dataclasses import dataclass

import psycopg

from .embed import Embedder

# Curated probe vocabulary for "what's the corpus made of". Mixed on purpose so
# the ranking is interesting: ubiquitous scenery, common road features, and
# rarer sights.
DEFAULT_CONCEPTS = [
    "blue sky", "clouds", "a road", "a highway", "trees", "a forest", "grass",
    "mountains", "hills", "a city", "buildings", "a bridge", "a tunnel",
    "water", "a lake", "the ocean", "a river", "the desert", "snow", "rain",
    "fog", "a sunset", "nighttime", "heavy traffic", "a truck", "a road sign",
    "a gas station", "a parking lot", "a farm field", "power lines", "a train",
    "roadwork and cones",
]


@dataclass
class Coverage:
    """How much of the corpus has been embedded for a model."""

    total_videos: int
    embedded_videos: int
    frames: int

    @property
    def remaining(self) -> int:
        """Videos not yet embedded."""
        return max(0, self.total_videos - self.embedded_videos)

    @property
    def pct(self) -> float:
        """Percent of the corpus embedded."""
        return 100.0 * self.embedded_videos / self.total_videos if self.total_videos else 0.0

    @property
    def frames_per_video(self) -> float:
        """Mean sampled frames per embedded video."""
        return self.frames / self.embedded_videos if self.embedded_videos else 0.0


@dataclass
class ConceptHit:
    """One probe word and how the corpus answered it."""

    concept: str
    matches: int
    best_sim: float


def coverage(conn: psycopg.Connection, model_id: str) -> Coverage:
    """Embedded-vs-total video counts (non-flagged) for `model_id`."""
    with conn.cursor() as cur:
        cur.execute("SELECT count(*) FROM videos WHERE NOT flagged")
        total = cur.fetchone()[0]
        cur.execute(
            "SELECT count(DISTINCT video_id), count(*) FROM frame_embeddings WHERE model = %s",
            (model_id,),
        )
        vids, frames = cur.fetchone()
    return Coverage(total_videos=total, embedded_videos=vids or 0, frames=frames or 0)


def db_size(conn: psycopg.Connection) -> dict:
    """frame_embeddings storage: total, table, and index bytes (+ pretty)."""
    with conn.cursor() as cur:
        cur.execute(
            """
            SELECT pg_total_relation_size('frame_embeddings'),
                   pg_relation_size('frame_embeddings'),
                   pg_indexes_size('frame_embeddings')
            """
        )
        total, table, indexes = cur.fetchone()
        cur.execute("SELECT pg_size_pretty(%s::bigint)", (total,))
        total_pretty = cur.fetchone()[0]
    return {
        "total_bytes": total,
        "total_pretty": total_pretty,
        "table_bytes": table,
        "index_bytes": indexes,
    }


def concept_scan(
    conn: psycopg.Connection,
    embedder: Embedder,
    model_id: str,
    concepts: list[str] | None = None,
    threshold: float = 0.1,
) -> list[ConceptHit]:
    """For each probe word, count frames at cosine similarity >= threshold.

    Returns hits sorted most→least common. One sequential scan per concept
    (cosine has no usable index for a range count), so this is an occasional
    check-in tool, not a hot path — fine while the table is modest.
    """
    concepts = concepts or DEFAULT_CONCEPTS
    max_dist = 1.0 - threshold
    out: list[ConceptHit] = []
    with conn.cursor() as cur:
        for c in concepts:
            q = embedder.embed_text(c)
            cur.execute(
                """
                SELECT count(*) FILTER (WHERE embedding <=> %(q)s <= %(md)s),
                       COALESCE(1 - min(embedding <=> %(q)s), 0)
                FROM frame_embeddings WHERE model = %(m)s
                """,
                {"q": q, "md": max_dist, "m": model_id},
            )
            matches, best = cur.fetchone()
            out.append(ConceptHit(concept=c, matches=int(matches), best_sim=float(best)))
    out.sort(key=lambda h: (h.matches, h.best_sim), reverse=True)
    return out
