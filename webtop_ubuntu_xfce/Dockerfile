# Pinned strictly to the user-specified arm64 manifest of linuxserver/webtop:ubuntu-xfce-76362218-ls10
FROM lscr.io/linuxserver/webtop:ubuntu-xfce-76362218-ls10@sha256:12b00cce8f60ee8f4553f2726d35c5d48d34d6c87513b1044a20619a0a2e3c3b

ARG BUILD_VERSION="dev"
ENV APP_VERSION="${BUILD_VERSION}" \
    START_DOCKER="false" \
    DISABLE_DRI3="true" \
    SELKIES_USE_CSS_SCALING="true" \
    LC_ALL="zh_TW.UTF-8" \
    LANG="zh_TW.UTF-8" \
    LANGUAGE="zh_TW" \
    DOWNLOAD_DIR="/share/downloads"

# Install complete Traditional Chinese fonts and language packs, then preserve original bwrap binary
RUN apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get install --no-install-recommends -y \
        fonts-noto-cjk \
        fonts-noto-cjk-extra \
        fonts-wqy-zenhei \
        language-pack-zh-hant \
        language-pack-gnome-zh-hant && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/* && \
    mv /usr/bin/bwrap /usr/bin/bwrap.real

# Copy Home Assistant options loader, fontconfig TC preference, bwrap fallback wrapper, and Chromium wrapper
COPY rootfs /

# Guarantee executable permissions regardless of host filesystem and refresh font cache
RUN chmod a+x \
    /defaults/startwm.sh \
    /etc/s6-overlay/s6-rc.d/init-ha-options/run \
    /etc/s6-overlay/s6-rc.d/init-ha-downloads/run \
    /usr/bin/bwrap \
    /usr/local/bin/wrapped-chromium && \
    fc-cache -fv

LABEL \
    org.opencontainers.image.title="Home Assistant App: Webtop (Ubuntu XFCE)" \
    org.opencontainers.image.description="Ubuntu XFCE desktop in your browser via Selkies (linuxserver/webtop)." \
    org.opencontainers.image.source="https://github.com/s90061/ha-apps" \
    org.opencontainers.image.licenses="GPL-3.0-only"
