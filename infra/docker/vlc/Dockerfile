FROM ubuntu:24.04

RUN export DEBIAN_FRONTEND=noninteractive \
  && apt-get update \
  && apt-get install -y --no-install-recommends \
    build-essential \
    ca-certificates \
    curl \
    fluxbox \
    git \
    grc \
    libvlc-dev \
    net-tools \
    pkg-config \
    supervisor \
    syslog-ng \
    x11vnc \
    xvfb \
    vim \
    vlc \
    vlc-plugin-base \
    x11-xserver-utils \
    xterm \
  && rm -rf /var/lib/apt/lists/*

ARG TARGETARCH
RUN curl -fsSL https://go.dev/dl/go1.21.13.linux-${TARGETARCH}.tar.gz \
  | tar -C /usr/local -xz
ENV PATH="/usr/local/go/bin:/go/bin:${PATH}"
ENV GOPATH="/go"

EXPOSE 5900
EXPOSE 8080

ENV VNC_PASSWD=123456

WORKDIR /go/src/github.com/adanalife/tripbot
COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN ln -s /go/src/github.com/adanalife/tripbot /opt/tripbot \
  && mkdir -p /opt/data

ARG VERSION=dev

WORKDIR /opt/tripbot
RUN go build -ldflags="-X main.version=${VERSION}" -o bin/vlc-server cmd/vlc-server/vlc-server.go

ENTRYPOINT ["/opt/tripbot/script/container_startup.sh"]
