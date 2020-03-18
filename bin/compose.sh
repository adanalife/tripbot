#!/usr/bin/env bash

# enable buildkit
# https://www.docker.com/blog/faster-builds-in-compose-thanks-to-buildkit-support/
export COMPOSE_DOCKER_CLI_BUILD=1
export DOCKER_BUILDKIT=1

docker-compose \
  -p danalol-stream \
  --project-directory . \
  --env-file infra/docker/env.docker \
  -f infra/docker/docker-compose.yml \
  -f infra/docker/docker-compose.development.yml \
  "$@"
