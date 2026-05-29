# onscreens-server: the HTTP API that owns overlay state (leaderboard, middle
# text, rotators, timewarp, GPS/flag images) and serves the OBS browser-source
# feeds. A pure-Go binary with no libvlc/GPU/X needs — modeled on the tripbot
# image, not the heavyweight vlc image it used to ride inside. GPS.png is
# embedded in the binary (pkg/onscreens-server), so the runtime needs no assets.

# Builder: produce a static binary with Go 1.26
FROM golang:1.26-bookworm AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

ARG VERSION=dev

COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=${VERSION}" -o /out/onscreens-server ./cmd/onscreens-server

# Runtime: minimal Ubuntu 24.04 matching the tripbot/vlc/obs containers
FROM ubuntu:24.04

RUN apt-get update \
  && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
    tini \
    ca-certificates \
    tzdata \
    curl \
  && rm -rf /var/lib/apt/lists/*

COPY --from=builder /out/onscreens-server /usr/local/bin/onscreens-server

ARG VERSION=dev
ARG SHA=unknown
RUN mkdir -p /etc/tripbot \
  && echo "${VERSION}" > /etc/tripbot/version \
  && echo "${SHA}" > /etc/tripbot/sha

EXPOSE 8081

HEALTHCHECK --interval=30s --timeout=5s --retries=3 CMD curl -fsS http://localhost:8081/health/live

ENTRYPOINT ["/usr/bin/tini", "--", "/usr/local/bin/onscreens-server"]
