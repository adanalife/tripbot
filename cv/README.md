# dashcam-cv

Visual search over the adanalife dashcam corpus. SigLIP2 image embeddings are
computed once over the (finite, immutable) corpus and stored in Postgres +
pgvector; a plain-text query (`sunset`, `welcome to nevada`) is embedded into the
same space and the nearest frames are returned. Think Google Image Search over
your own footage — and because the model is **SigLIP2 NaFlex**, it reads sign
text and keeps the full 16:9 widescreen frame (no square crop), so road-trip
queries like place names and signs work.

Self-contained Python sub-tree inside the tripbot repo — shares the `videos`
table and `db/migrate/` but builds and ships independently of the Go code. Full
design + rationale: `vault/tripbot/dashcam-cv-plan.md`.

## Model

Default: **`google/siglip2-so400m-patch16-naflex`** (the flagship SigLIP2, 1152-dim,
non-gated) via HuggingFace `transformers`. NaFlex preserves native aspect ratio
(a 1920×1080 frame → a ~21×12 landscape patch grid), so widescreen edge detail
survives. The `frame_embeddings.model` column records the checkpoint so a future
swap can coexist with old vectors.

> **so400m is ~1.1B params (~4.4 GB fp32) — it needs real RAM.** It runs on the
> mini-PC (32 GB) and in Docker on any ≥16 GB host, but **cannot run on the 8 GB
> dev Mac** (loading it crashes even a 7 GB colima VM). On this Mac, the corpus
> embedding is a mini-PC job; local work is code/build/CLI/DB. `DASHCAM_CV_DTYPE=bfloat16`
> roughly halves memory on a constrained box; `google/siglip2-base-patch16-naflex`
> (768-dim) is a lighter NaFlex option if you set a matching `vector(768)` column.

## Toolchain (mise + uv, run in Docker)

Latest PyTorch ships no macOS x86_64 wheel, so on an **Intel Mac the tool runs in
Docker**, not via a host `uv sync`. mise still provides python + uv (pinned in
`.tool-versions`) for `uv lock` and ruff; the model runs in the container. On
linux / Apple-Silicon, `mise exec -- uv sync && uv run dashcam-cv …` works
natively.

```bash
docker build -t dashcam-cv:dev cv/          # bakes the so400m checkpoint (large)
docker build --build-arg BAKE_MODEL=0 -t dashcam-cv:dev cv/   # fast build, model at runtime
```

## Local dev — no AWS / Secrets Manager

Everything runs against a plain local Postgres with pgvector — **no aws-vault, no
ESO, no k3d.** The docker-compose `db` service image is `pgvector/pgvector:pg16`.

```bash
# from the tripbot repo root
ENV=development bin/devenv up -d db          # pgvector postgres (uses env.docker creds)
ENV=development bin/devenv up migrate        # runs db/migrate up (incl. frame_embeddings)
# seed the videos table (runs inside the container; host needs no psql):
docker exec -i $(docker compose ps -q db) psql -U tripbot_docker -d tripbot_docker \
  -c "\copy videos FROM STDIN DELIMITER ',' CSV HEADER" < db/seed/videos.csv
```

## Usage (Docker, on the compose network)

```bash
NET=$(docker network ls --format '{{.Name}}' | grep -m1 default)   # the compose net
docker run --rm --network "$NET" \
  -e DATABASE_HOST=db -e DATABASE_USER=tripbot_docker -e DATABASE_PASS=hunter2 -e DATABASE_DB=tripbot_docker \
  -e DASHCAM_CV_CORPUS_DIR=/corpus \
  -v "$PWD/assets/video:/corpus:ro" -v dashcam-cv-hf:/opt/models \
  dashcam-cv:dev embed --all --limit 1 --apply        # dry-run without --apply
docker run --rm --network "$NET" -e DATABASE_HOST=db -e DATABASE_USER=tripbot_docker \
  -e DATABASE_PASS=hunter2 -e DATABASE_DB=tripbot_docker \
  dashcam-cv:dev find "golden gate bridge" -k 5
```

`embed` is dry-run by default (counts frames, writes nothing); `--apply` persists.
`find` supports `--state <US state>` (location filter via the videos join — the
embeddings stay purely visual).

## Layout

| file | role |
|---|---|
| `db.py` | psycopg connection + pgvector registration (`DATABASE_*` env) |
| `corpus.py` | walk the corpus dir, join files to `videos` rows by slug |
| `frames.py` | sample frames at `--interval` (PyAV, in-memory) |
| `embed.py` | transformers SigLIP2 NaFlex wrapper (image/text → vector) |
| `pipeline.py` | `embed_video()` — the atomic, idempotent decode→embed→upsert unit |
| `search.py` | text query → cosine top-K from `frame_embeddings` (+ `--state`) |
| `cli.py` | `dashcam-cv embed …` / `dashcam-cv find …` |
