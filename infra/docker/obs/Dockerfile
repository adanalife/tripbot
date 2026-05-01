FROM ubuntu:24.04

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y --no-install-recommends \
      obs-studio \
      xvfb \
      x11vnc \
      fluxbox \
      tini \
      gettext-base \
      procps \
      x11-utils \
      ca-certificates \
      dbus-x11 \
      libgl1-mesa-dri \
      fonts-dejavu-core \
    && rm -rf /var/lib/apt/lists/*

COPY configs/obs-studio/docker/ /opt/obs/
RUN chmod +x /opt/obs/entrypoint.sh /opt/obs/healthcheck.sh

EXPOSE 5900

HEALTHCHECK --interval=10s --timeout=3s --retries=3 CMD /opt/obs/healthcheck.sh

ENTRYPOINT ["/usr/bin/tini", "--", "/opt/obs/entrypoint.sh"]
