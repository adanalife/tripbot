#!/usr/bin/env bash

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

psql "$DSN" -c "TRUNCATE videos RESTART IDENTITY CASCADE; \copy videos FROM '/seed/videos.csv' DELIMITER ',' CSV HEADER;"
