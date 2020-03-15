#!/usr/bin/env bash

#TODO: if cmd/tripbot/Dockerfile doesnt exist, exit early

# docker build -t tripbot:latest . -f cmd/tripbot/Dockerfile
docker-compose --project-directory . -f cmd/tripbot/docker-compose.yml --env-file .env.staging build
