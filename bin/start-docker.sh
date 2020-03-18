#!/usr/bin/env bash

#TODO: if cmd/tripbot/Dockerfile doesnt exist, exit early

# docker build -t tripbot:latest . -f cmd/tripbot/Dockerfile
docker-compose \
  -p danalol-stream \
  --project-directory . \
  -f infra/docker/docker-compose.yml \
  -f infra/docker/docker-compose.development.yml \
  up
