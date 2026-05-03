FROM golang:1.21-bookworm

RUN apt-get update \
  && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
    libvlc-dev \
  && rm -rf /var/lib/apt/lists/*
