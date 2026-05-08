# Builder: produce a static-ish tripbot binary with Go 1.21
FROM golang:1.26-bookworm AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/tripbot ./cmd/tripbot

# Runtime: minimal Ubuntu 24.04 matching the VLC/OBS containers
FROM ubuntu:24.04

RUN apt-get update \
  && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
    tini \
    ca-certificates \
    tzdata \
    curl \
  && rm -rf /var/lib/apt/lists/*

COPY --from=builder /out/tripbot /usr/local/bin/tripbot

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --retries=3 CMD curl -fsS http://localhost:8080/health/live

ENTRYPOINT ["/usr/bin/tini", "--", "/usr/local/bin/tripbot"]
