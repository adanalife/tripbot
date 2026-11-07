# The schema image: the migrate CLI, db/migrate and the seed script, with no
# bot binary. The migrate initContainer and the seed Job run from it, tagged
# with the tripbot release they ship beside, so a schema change ships without
# rebuilding the bot and a bot in any language applies the same SQL.
FROM ghcr.io/adanalife/mirror/ubuntu:24.04

RUN apt-get update \
  && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
    ca-certificates \
    postgresql-client \
  && rm -rf /var/lib/apt/lists/*

COPY --from=ghcr.io/adanalife/mirror/migrate:4 /usr/local/bin/migrate /usr/local/bin/migrate
COPY db/migrate /migrations

# Seed script + data so the k8s seed Job is self-contained.
COPY infra/docker/bin/seed-db.sh /usr/local/bin/seed-db
COPY db/seed /seed

# Stamped for the release workflow's --version check (verify-stamped-image.sh).
ARG VERSION=dev
ARG SHA=unknown
ENV IMAGE_VERSION=${VERSION} IMAGE_SHA=${SHA}

COPY infra/docker/bin/migrate-entrypoint.sh /usr/local/bin/entrypoint
ENTRYPOINT ["/usr/local/bin/entrypoint"]
