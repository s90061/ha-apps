# Pinned strictly to the user-specified arm64 manifest of linuxserver/webtop:ubuntu-xfce-76362218-ls10
FROM lscr.io/linuxserver/webtop:ubuntu-xfce-76362218-ls10@sha256:12b00cce8f60ee8f4553f2726d35c5d48d34d6c87513b1044a20619a0a2e3c3b

ARG BUILD_VERSION="dev"
ENV APP_VERSION="${BUILD_VERSION}" \
    START_DOCKER="false" \
    DISABLE_DRI3="true" \
    SELKIES_USE_CSS_SCALING="true"

# Preserve original bwrap binary before copying wrapper
RUN mv /usr/bin/bwrap /usr/bin/bwrap.real

# Copy Home Assistant options loader, bwrap fallback wrapper, and Chromium wrapper
COPY rootfs /

# Guarantee executable permissions regardless of host filesystem
RUN chmod a+x \
    /defaults/startwm.sh \
    /etc/s6-overlay/s6-rc.d/init-ha-options/run \
    /usr/bin/bwrap \
    /usr/local/bin/wrapped-chromium

LABEL \
    org.opencontainers.image.title="Home Assistant App: Webtop (Ubuntu XFCE)" \
    org.opencontainers.image.description="Ubuntu XFCE desktop in your browser via Selkies (linuxserver/webtop)." \
    org.opencontainers.image.source="https://github.com/s90061/ha-apps" \
    org.opencontainers.image.licenses="GPL-3.0-only"
