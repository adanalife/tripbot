"""Text query -> nearest frames by cosine distance (pgvector)."""

from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime

import psycopg

from .embed import Embedder

# Embeddings stay purely visual; place/time are structured filters on the videos
# join (state + date_filmed are per-video). This is how "!find sunset in nevada"
# or "!find construction in california" work without touching the vector space —
# the query parser (query.py) extracts the facets, the vector ranks the rest.
# A NULL array means "no constraint on this axis", so each filter is independent.
_SEARCH_SQL = """
SELECT fe.video_id, v.slug, fe.ts_sec, fe.embedding <=> %(q)s AS distance,
       v.state, v.date_filmed
FROM frame_embeddings fe
JOIN videos v ON v.id = fe.video_id
WHERE fe.model = %(model)s
  AND (%(states)s::text[] IS NULL OR lower(v.state) = ANY(%(states)s::text[]))
  AND (%(months)s::int[] IS NULL OR extract(month FROM v.date_filmed)::int = ANY(%(months)s::int[]))
ORDER BY fe.embedding <=> %(q)s
LIMIT %(k)s
"""


@dataclass
class SearchHit:
    """One ranked search result: a frame, its video, and its distance."""

    video_id: int
    slug: str
    ts_sec: float
    distance: float
    state: str | None
    date_filmed: datetime | None = None

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
    states: list[str] | None = None,
    months: list[int] | None = None,
) -> list[SearchHit]:
    """Embed `query` with the same model and return the k nearest frames.

    Filters by model so vectors from different checkpoints never get compared in
    one ranking (the frame_embeddings.model forward-compat column). Optionally
    restricts to one or more US states (case-insensitive) and/or a set of film
    months (1-12) via the videos join — the structured half of hybrid search.

    `state` is the legacy single-state convenience; it's folded into `states`.
    A None filter means "no constraint on that axis".
    """
    vec = embedder.embed_text(query)
    all_states = list(states or [])
    if state and state not in all_states:
        all_states.append(state)
    # Lowercased for the case-insensitive ANY() compare; None disables the filter.
    states_param = [s.lower() for s in all_states] or None
    months_param = list(months) if months else None
    with conn.cursor() as cur:
        cur.execute(
            _SEARCH_SQL,
            {
                "q": vec,
                "model": model_id or embedder.model_id,
                "k": k,
                "states": states_param,
                "months": months_param,
            },
        )
        return [
            SearchHit(
                video_id=r[0],
                slug=r[1],
                ts_sec=float(r[2]),
                distance=float(r[3]),
                state=r[4],
                date_filmed=r[5],
            )
            for r in cur.fetchall()
        ]
