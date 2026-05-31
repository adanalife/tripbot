#!/usr/bin/env bash
# Called by infra/docker/docker-compose.yml (seed service entrypoint) and baked
# into the tripbot image (infra/docker/tripbot/Dockerfile) as /usr/local/bin/seed
# for the k8s one-shot DB seed Job. Idempotently loads db/seed/videos.csv.

set -ex

apt-get update && apt-get install -y --no-install-recommends postgresql-client

DSN="postgres://$DATABASE_USER:$DATABASE_PASS@$DATABASE_HOST/$DATABASE_DB"

# Skip-or-clobber: a fresh cluster races tripbot's first LoadOrCreate against
# this seed. Tripbot-inserted rows always have flagged=true (lat/lng=0; see
# pkg/video/db.go save()), so if every existing row is flagged we know the
# table only holds race-loss placeholders, not real seeded data — clobber and
# reseed. If any row has flagged=false, the CSV import is already done.
REAL=$(psql "$DSN" -tAc "SELECT COUNT(*) FROM videos WHERE NOT flagged;")
if [ "$REAL" -gt 0 ]; then
  echo "videos table already has $REAL unflagged rows — skipping seed"
  exit 0
fi

# Heredoc rather than -c because \copy is a psql meta-command and can't
# share a -c string with SQL. --single-transaction so a failed COPY rolls
# the TRUNCATE back instead of leaving the table empty.
psql "$DSN" --single-transaction <<'EOF'
TRUNCATE videos RESTART IDENTITY CASCADE;
\copy videos FROM '/seed/videos.csv' DELIMITER ',' CSV HEADER;
EOF
