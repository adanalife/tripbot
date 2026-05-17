FROM ubuntu:24.04

RUN export DEBIAN_FRONTEND=noninteractive \
  && apt-get update \
  && apt-get install -y --no-install-recommends \
    build-essential \
    ca-certificates \
    curl \
    git \
    grc \
    libvlc-dev \
    net-tools \
    pkg-config \
    supervisor \
    syslog-ng \
    vim \
    vlc-plugin-base \
  && rm -rf /var/lib/apt/lists/*

ARG TARGETARCH
RUN curl -fsSL https://go.dev/dl/go1.26.3.linux-${TARGETARCH}.tar.gz \
  | tar -C /usr/local -xz
ENV PATH="/usr/local/go/bin:/go/bin:${PATH}"
ENV GOPATH="/go"

EXPOSE 8080 8081

# Static configs baked into the image; the entrypoint no longer regenerates
# these via heredocs at boot.
COPY infra/docker/vlc/config/syslog.conf /etc/supervisor/conf.d/syslog.conf
COPY infra/docker/vlc/config/vlc.conf /etc/supervisor/conf.d/vlc.conf
COPY infra/docker/vlc/config/onscreens-server.conf /etc/supervisor/conf.d/onscreens-server.conf

WORKDIR /go/src/github.com/adanalife/tripbot
COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN ln -s /go/src/github.com/adanalife/tripbot /opt/tripbot \
  && mkdir -p /opt/data

ARG VERSION=dev
ARG SHA=unknown

WORKDIR /opt/tripbot
RUN go build -ldflags="-X main.version=${VERSION}" -o bin/vlc-server ./cmd/vlc-server
RUN go build -ldflags="-X main.version=${VERSION}" -o bin/onscreens-server ./cmd/onscreens-server

RUN mkdir -p /etc/tripbot \
  && echo "${VERSION}" > /etc/tripbot/version \
  && echo "${SHA}" > /etc/tripbot/sha

ENTRYPOINT ["/opt/tripbot/script/container_startup.sh"]
