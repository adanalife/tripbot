FROM ubuntu:24.04

ENV DEBIAN_FRONTEND=noninteractive

# msttcorefonts is gated on a EULA prompt; pre-accept it before apt-get so the
# install stays unattended. Trebuchet MS is needed by the OBS browser-source
# onscreen pages (CSS font-family).
#
# obs-studio comes from the official PPA (ppa:obsproject/obs-studio) — Ubuntu's
# universe build of obs-studio omits CEF, so `browser_source` registers as
# "Source ID 'browser_source' not found" at scene load. The PPA build bundles
# CEF and registers the plugin.
RUN echo "ttf-mscorefonts-installer msttcorefonts/accepted-mscorefonts-eula select true" \
      | debconf-set-selections \
    && apt-get update && apt-get install -y --no-install-recommends \
      software-properties-common \
      gnupg \
      ca-certificates \
    && add-apt-repository -y ppa:obsproject/obs-studio \
    && apt-get update && apt-get install -y --no-install-recommends \
      obs-studio \
      xvfb \
      x11vnc \
      fluxbox \
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
