FROM ubuntu:24.04

# libva2 / libva-drm2 / mesa-va-drivers / vainfo are the userspace
# VA-API plumbing libvlc needs to discover and use /dev/dri/renderD128
# when VLC_AVCODEC_HW is "any" or "vaapi". intel-media-va-driver-non-free
# is the actual Intel-specific driver and is amd64-only on Ubuntu
# multiverse, so installed conditionally.
ARG TARGETARCH
RUN export DEBIAN_FRONTEND=noninteractive \
  && apt-get update \
  && apt-get install -y --no-install-recommends \
    build-essential \
    ca-certificates \
    curl \
    git \
    grc \
    libva-drm2 \
    libva2 \
    libvlc-dev \
    mesa-va-drivers \
    net-tools \
    pkg-config \
    supervisor \
    syslog-ng \
    vainfo \
    vim \
    vlc-plugin-base \
  && if [ "$TARGETARCH" = "amd64" ]; then \
       apt-get install -y --no-install-recommends intel-media-va-driver-non-free; \
     fi \
  && rm -rf /var/lib/apt/lists/*

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
