"""Text query -> nearest frames by cosine distance (pgvector)."""

from __future__ import annotations

from dataclasses import dataclass

import psycopg

from .embed import Embedder

# Embeddings stay purely visual; location is a structured filter on the videos
# join (state is per-video). This is how "!find sunset in nevada" works without
# touching the vector space — see the GPS discussion in the plan.
_SEARCH_SQL = """
SELECT fe.video_id, v.slug, fe.ts_sec, fe.embedding <=> %(q)s AS distance, v.state
FROM frame_embeddings fe
JOIN videos v ON v.id = fe.video_id
WHERE fe.model = %(model)s
  AND (%(state)s::text IS NULL OR lower(v.state) = lower(%(state)s::text))
ORDER BY fe.embedding <=> %(q)s
LIMIT %(k)s
"""


@dataclass
class SearchHit:
    video_id: int
    slug: str
    ts_sec: float
    distance: float
    state: str | None

    @property
    def similarity(self) -> float:
        """Cosine similarity in [-1, 1] (pgvector <=> is 1 - cosine_sim)."""
        return 1.0 - self.distance


def search(
    conn: psycopg.Connection,
    embedder: Embedder,
    query: str,
    k: int = 10,
    model_id: str | None = None,
    state: str | None = None,
) -> list[SearchHit]:
    """Embed `query` with the same model and return the k nearest frames.

    Filters by model so vectors from different checkpoints never get compared in
    one ranking (the frame_embeddings.model forward-compat column). Optionally
    restricts to a US state (case-insensitive) via the videos join.
    """
    vec = embedder.embed_text(query)
    with conn.cursor() as cur:
        cur.execute(
            _SEARCH_SQL,
            {"q": vec, "model": model_id or embedder.model_id, "k": k, "state": state},
        )
        return [
            SearchHit(
                video_id=r[0], slug=r[1], ts_sec=float(r[2]), distance=float(r[3]), state=r[4]
            )
            for r in cur.fetchall()
        ]
