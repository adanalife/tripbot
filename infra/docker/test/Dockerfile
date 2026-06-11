# Image used by the `test` service in docker-compose.testing.yml to
# run `go test`. libvlc-dev is required because pkg/vlc-server links
# against libvlc via the github.com/adrg/libvlc-go CGO bindings — the
# tripbot binary builds with CGO_ENABLED=0 (so its image doesn't need
# this), and vlc/Dockerfile does have it but is heavyweight (X server,
# supervisor, the VLC daemon itself). Easier to keep tests on a small
# purpose-built image.
FROM ghcr.io/adanalife/mirror/golang:1.26-bookworm

RUN apt-get update \
  && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
    libvlc-dev \
  && rm -rf /var/lib/apt/lists/*
