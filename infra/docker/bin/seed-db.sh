#!/usr/bin/env bash

set -eix

apt update
apt install -y postgresql
psql "postgres://$DATABASE_USER:$DATABASE_PASS@$DATABASE_HOST/$DATABASE_DB" \
  -c "\copy videos FROM 'db/seed/videos.csv' DELIMITER ',' CSV HEADER;"
