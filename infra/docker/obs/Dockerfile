FROM ubuntu:24.04

ENV DEBIAN_FRONTEND=noninteractive

# OBS PPA ships the CEF-bundled obs-studio for amd64. Universe's obs-studio
# is the +dfsg build (no CEF), which silently breaks browser_source at
# scene-load time. PPA only publishes amd64; arm64 takes a different path
# (see Dockerfile.arm64, which compiles from source against the aarch64 CEF
# tarball).
#
# msttcorefonts wants EULA pre-acceptance for unattended install. Trebuchet
# MS is referenced by onscreen text sources.
RUN echo "ttf-mscorefonts-installer msttcorefonts/accepted-mscorefonts-eula select true" \
      | debconf-set-selections \
    && apt-get update \
    && apt-get install -y --no-install-recommends \
         software-properties-common ca-certificates gnupg \
    && add-apt-repository -y ppa:obsproject/obs-studio \
    && apt-get install -y --no-install-recommends \
         obs-studio \
         xvfb \
         x11vnc \
         fluxbox \
         feh \
         tini \
         gettext-base \
         procps \
         x11-utils \
         dbus-x11 \
         libgl1-mesa-dri \
         fonts-dejavu-core \
         ttf-mscorefonts-installer \
    && rm -rf /var/lib/apt/lists/*

COPY infra/docker/obs/config/ /opt/obs/config/
COPY infra/docker/obs/entrypoint.sh /opt/obs/entrypoint.sh
COPY infra/docker/obs/healthcheck.sh /opt/obs/healthcheck.sh
RUN chmod +x /opt/obs/entrypoint.sh /opt/obs/healthcheck.sh

COPY assets/twitch-overlay-left.png assets/twitch-overlay-right.png /opt/tripbot/assets/

EXPOSE 5900

ENV VNC_PASSWD=123456

HEALTHCHECK --interval=10s --timeout=3s --retries=3 CMD /opt/obs/healthcheck.sh

ENTRYPOINT ["/usr/bin/tini", "--", "/opt/obs/entrypoint.sh"]
