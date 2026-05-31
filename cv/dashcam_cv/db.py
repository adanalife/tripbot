"""Postgres connection + pgvector registration.

Reads the project-wide DATABASE_* env vars (same names tripbot's Go code and
cmd/backfill-miles use): DATABASE_USER, DATABASE_PASS, DATABASE_HOST,
DATABASE_DB, and an optional DATABASE_PORT (default 5432).

No AWS / Secrets Manager involved — the local dev loop points these at the
docker-compose `db` service (tripbot_docker / hunter2). See cv/README.md.
"""

from __future__ import annotations

import os

import psycopg
from pgvector.psycopg import register_vector

# Image-embedding width. Fixed by the production model (SigLIP2 so400m NaFlex,
# 1152-dim) and the frame_embeddings.embedding vector(1152) column. A model swap
# to a different-width checkpoint requires a new migration; same-width swaps don't.
EMBED_DIM = 1152


def dsn() -> str:
    """Build the libpq DSN from DATABASE_* env vars."""
    user = os.environ.get("DATABASE_USER", "tripbot_docker")
    password = os.environ.get("DATABASE_PASS", "hunter2")
    host = os.environ.get("DATABASE_HOST", "localhost")
    port = os.environ.get("DATABASE_PORT", "5432")
    name = os.environ.get("DATABASE_DB", "tripbot_docker")
    return f"postgresql://{user}:{password}@{host}:{port}/{name}?sslmode=disable"


def connect() -> psycopg.Connection:
    """Open a connection with the pgvector type adapter registered.

    Registering the adapter lets psycopg pass numpy arrays straight into
    vector(N) columns and read them back as numpy arrays.
    """
    conn = psycopg.connect(dsn())
    register_vector(conn)
    return conn
