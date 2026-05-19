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
#
# intel-media-va-driver-non-free + vainfo enable hardware H.264/H.265
# encode via VAAPI on Intel iGPU hosts (Gen 11+ Iris Xe / UHD 770).
# OBS connects to the iGPU's render engine for OpenGL composite via the
# sway Wayland compositor (WLR_BACKENDS=headless), so /dev/dri/card0 must
# be mounted at runtime — provided by the Intel device plugin's
# gpu.intel.com/i915 resource grant in the prod-1 overlay.
#
# Display stack: sway (headless Wayland compositor) + wayvnc (Wayland VNC
# server) + qt6-wayland (Qt's Wayland QPA plugin for OBS's UI). Replaces
# the previous Xvfb / fluxbox / x11vnc trio so OBS's OpenGL hits the iGPU
# instead of falling back to Mesa llvmpipe (CPU rasterizer).
#
# supervisor manages the three (+ the browser-refresh loop) so each has
# its own restart policy and dependency-on-Wayland-socket logic; matches
# the VLC container's structure.
RUN echo "ttf-mscorefonts-installer msttcorefonts/accepted-mscorefonts-eula select true" \
      | debconf-set-selections \
    && apt-get update \
    && apt-get install -y --no-install-recommends \
         software-properties-common ca-certificates gnupg \
    && add-apt-repository -y ppa:obsproject/obs-studio \
    && apt-get install -y --no-install-recommends \
         obs-studio \
         sway \
         wayvnc \
         xwayland \
         qt6-wayland \
         supervisor \
         feh \
         tini \
         gettext-base \
         procps \
         libgl1-mesa-dri \
         intel-media-va-driver-non-free \
         vainfo \
         fonts-dejavu-core \
         ttf-mscorefonts-installer \
         python3 \
         python3-venv \
    && rm -rf /var/lib/apt/lists/*

# obsws-python (in a venv to stay clear of PEP 668 system-managed python)
# powers the hourly browser-source refresh that the entrypoint kicks off
# as a workaround for CEF's per-frame memory leak.
RUN python3 -m venv /opt/obs/venv \
    && /opt/obs/venv/bin/pip install --no-cache-dir obsws-python

COPY infra/docker/obs/config/ /opt/obs/config/
COPY infra/docker/obs/script/ /opt/obs/script/
COPY infra/docker/obs/entrypoint.sh /opt/obs/entrypoint.sh
COPY infra/docker/obs/healthcheck.sh /opt/obs/healthcheck.sh
COPY bin/obs-browser-refresh /opt/obs/bin/obs-browser-refresh
RUN chmod +x /opt/obs/entrypoint.sh /opt/obs/healthcheck.sh \
             /opt/obs/bin/obs-browser-refresh /opt/obs/script/*.sh

# Per-program supervisor configs live in /etc/supervisor/conf.d/. The
# default supervisord.conf from the package globs *.conf out of that dir.
COPY infra/docker/obs/supervisor/ /etc/supervisor/conf.d/

COPY assets/twitch-overlay-left.png assets/twitch-overlay-right.png /opt/tripbot/assets/

ARG VERSION=dev
ARG SHA=unknown
RUN mkdir -p /etc/tripbot \
  && echo "${VERSION}" > /etc/tripbot/version \
  && echo "${SHA}" > /etc/tripbot/sha

EXPOSE 5900
EXPOSE 4455

ENV VNC_PASSWD=123456

HEALTHCHECK --interval=10s --timeout=3s --retries=3 CMD /opt/obs/healthcheck.sh

ENTRYPOINT ["/usr/bin/tini", "--", "/opt/obs/entrypoint.sh"]
