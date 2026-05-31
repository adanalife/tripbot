-- Visual-search frame embeddings for the dashcam corpus (see tripbot/cv/ and
-- vault/tripbot/dashcam-cv-plan.md). Requires the pgvector extension, which the
-- postgres image provides (pgvector/pgvector:pg16).
CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE frame_embeddings (
  id           BIGSERIAL PRIMARY KEY,
  video_id     INTEGER NOT NULL REFERENCES videos(id),
  ts_sec       FLOAT   NOT NULL,            -- timestamp within the video (seconds)
  embedding    vector(1152) NOT NULL,       -- SigLIP2 so400m NaFlex image embedding (1152-dim)
  model        VARCHAR(64) NOT NULL,        -- e.g. 'transformers:google/siglip2-so400m-patch16-naflex'
  date_created TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Idempotent re-runs: (video_id, ts_sec, model) is unique so ON CONFLICT DO
-- NOTHING makes embed_video() safe to re-run / overlap with future pipeline ingest.
CREATE UNIQUE INDEX frame_embeddings_dedupe ON frame_embeddings (video_id, ts_sec, model);

-- Approximate-nearest-neighbour index for cosine search. Built here for
-- correctness; for the corpus-wide batch, prefer creating it AFTER the bulk
-- insert (much faster) — drop+recreate around the load if needed.
CREATE INDEX frame_embeddings_ann ON frame_embeddings USING hnsw (embedding vector_cosine_ops);
