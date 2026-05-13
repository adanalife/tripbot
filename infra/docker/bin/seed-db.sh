#!/usr/bin/env bash

set -ex

apt-get install -y --no-install-recommends postgresql-client

DSN="postgres://$DATABASE_USER:$DATABASE_PASS@$DATABASE_HOST/$DATABASE_DB"

COUNT=$(psql "$DSN" -tAc "SELECT COUNT(*) FROM videos;")
if [ "$COUNT" -gt 0 ]; then
  echo "videos table already has $COUNT rows — skipping seed"
  exit 0
fi

psql "$DSN" -c "\copy videos FROM '/seed/videos.csv' DELIMITER ',' CSV HEADER;"
