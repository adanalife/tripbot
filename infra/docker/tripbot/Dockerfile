# Builder: produce a static-ish tripbot binary with Go 1.26
FROM golang:1.26-bookworm AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

ARG VERSION=dev

COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=${VERSION}" -o /out/tripbot ./cmd/tripbot

# Runtime: minimal Ubuntu 24.04 matching the VLC/OBS containers
FROM ubuntu:24.04

RUN apt-get update \
  && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
    tini \
    ca-certificates \
    tzdata \
    curl \
    postgresql-client \
  && rm -rf /var/lib/apt/lists/*

COPY --from=builder /out/tripbot /usr/local/bin/tripbot

# Bundle the migrate CLI + db/migrate so this image can run schema migrations.
COPY --from=migrate/migrate:4 /usr/local/bin/migrate /usr/local/bin/migrate
COPY db/migrate /migrations

# Bundle seed script + data so the k8s seed Job is self-contained.
COPY infra/docker/bin/seed-db.sh /usr/local/bin/seed-db
COPY db/seed /seed

ARG VERSION=dev
ARG SHA=unknown
RUN mkdir -p /etc/tripbot \
  && echo "${VERSION}" > /etc/tripbot/version \
  && echo "${SHA}" > /etc/tripbot/sha

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --retries=3 CMD curl -fsS http://localhost:8080/health/live

ENTRYPOINT ["/usr/bin/tini", "--", "/usr/local/bin/tripbot"]
