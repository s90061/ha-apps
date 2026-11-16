# Pinned strictly to the user-specified arm64 manifest of linuxserver/webtop:ubuntu-xfce (76362218-ls10)
FROM lscr.io/linuxserver/webtop@sha256:12b00cce8f60ee8f4553f2726d35c5d48d34d6c87513b1044a20619a0a2e3c3b

ARG BUILD_VERSION="dev"
ENV APP_VERSION="${BUILD_VERSION}" \
    START_DOCKER="false"

# Copy Home Assistant options loader and Chromium wrapper
COPY rootfs /

# Guarantee executable permissions regardless of host filesystem
RUN chmod a+x \
    /etc/s6-overlay/s6-rc.d/init-ha-options/run \
    /usr/local/bin/wrapped-chromium

LABEL \
    org.opencontainers.image.title="Home Assistant App: Webtop (Ubuntu XFCE)" \
    org.opencontainers.image.description="Ubuntu XFCE desktop in your browser via Selkies (linuxserver/webtop)." \
    org.opencontainers.image.source="https://github.com/s90061/ha-apps" \
    org.opencontainers.image.licenses="GPL-3.0-only"
