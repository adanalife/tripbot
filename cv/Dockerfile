# dashcam-cv batch image. Built + released independently of the Go tripbot
# image (path-filtered on cv/**). CPU-only (torch 2.x+cpu); the SigLIP2 NaFlex
# checkpoint is baked in so the corpus-wide batch Job has no network dependency
# at runtime.
#
# Builds clean on linux/amd64 (local dev + mini-PC). Multi-arch is a
# release-phase concern (the plan's Phase 3.5).
FROM python:3.13-slim AS base

# uv pinned to match cv/.tool-versions.
COPY --from=ghcr.io/astral-sh/uv:0.11.17 /uv /uvx /bin/

ENV UV_COMPILE_BYTECODE=1 \
    UV_LINK_MODE=copy \
    # bake model weights here and point HuggingFace at it at runtime too.
    HF_HOME=/opt/models \
    UV_PROJECT_ENVIRONMENT=/opt/venv

WORKDIR /app

# Dependency layer — cached unless the lockfile changes.
COPY pyproject.toml uv.lock README.md ./
RUN uv sync --frozen --no-dev --no-install-project

# App layer.
COPY dashcam_cv ./dashcam_cv
RUN uv sync --frozen --no-dev

ENV PATH="/opt/venv/bin:$PATH"
# Unbuffered stdout so per-video embed lines stream to `kubectl logs` live
# (placed after the deps layer to keep the torch install cached).
ENV PYTHONUNBUFFERED=1

# Bake the SigLIP2 NaFlex checkpoint into the image (so the batch Job needs no
# network at runtime). Set --build-arg BAKE_MODEL=0 for fast dev/CI builds that
# download the model at runtime instead.
ARG DASHCAM_CV_MODEL=google/siglip2-so400m-patch16-naflex
ARG BAKE_MODEL=1
RUN if [ "$BAKE_MODEL" = "1" ]; then \
      python -c "from transformers import AutoModel, AutoProcessor; m='${DASHCAM_CV_MODEL}'; AutoModel.from_pretrained(m); AutoProcessor.from_pretrained(m)"; \
    fi

ENTRYPOINT ["dashcam-cv"]
CMD ["--help"]
