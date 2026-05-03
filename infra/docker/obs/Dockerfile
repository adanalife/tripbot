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

COPY infra/docker/obs/config/ /opt/obs/config/
COPY infra/docker/obs/entrypoint.sh /opt/obs/entrypoint.sh
COPY infra/docker/obs/healthcheck.sh /opt/obs/healthcheck.sh
RUN chmod +x /opt/obs/entrypoint.sh /opt/obs/healthcheck.sh

COPY assets/twitch-overlay-left.png assets/twitch-overlay-right.png /opt/tripbot/assets/

# Bake dummy onscreens into a defaults dir so the runtime named volume mount
# at /opt/data/run/ doesn't shadow them. The entrypoint seeds the volume from
# here on first boot when INCLUDE_DUMMY_ONSCREENS is true. Off by default;
# enable for debugging OBS without a live bot:
#   docker compose build --build-arg INCLUDE_DUMMY_ONSCREENS=true obs
ARG INCLUDE_DUMMY_ONSCREENS=false
ENV INCLUDE_DUMMY_ONSCREENS=${INCLUDE_DUMMY_ONSCREENS}
COPY infra/docker/obs/dummy-onscreens/ /opt/obs-dummy-defaults-src/
RUN if [ "${INCLUDE_DUMMY_ONSCREENS}" = "true" ]; then \
      mv /opt/obs-dummy-defaults-src /opt/obs-dummy-defaults; \
    else \
      rm -rf /opt/obs-dummy-defaults-src; \
    fi

EXPOSE 5900

ENV VNC_PASSWD=123456

HEALTHCHECK --interval=10s --timeout=3s --retries=3 CMD /opt/obs/healthcheck.sh

ENTRYPOINT ["/usr/bin/tini", "--", "/opt/obs/entrypoint.sh"]
