# Pinned strictly to the user-specified arm64 manifest of linuxserver/webtop:ubuntu-xfce-76362218-ls10
FROM lscr.io/linuxserver/webtop:ubuntu-xfce-76362218-ls10@sha256:12b00cce8f60ee8f4553f2726d35c5d48d34d6c87513b1044a20619a0a2e3c3b

ARG BUILD_VERSION="dev"
ARG WORDCRAFT_VERSION="0.18.1"
ARG WORDCRAFT_DEB_SHA256="a6e7558d9b9c436c9b71df746af2bb36d8e498452af65115d17fddeeb28a9522"
ENV APP_VERSION="${BUILD_VERSION}" \
    START_DOCKER="false" \
    DISABLE_DRI3="true" \
    SELKIES_USE_CSS_SCALING="true" \
    LC_ALL="zh_TW.UTF-8" \
    LANG="zh_TW.UTF-8" \
    LANGUAGE="zh_TW" \
    DOWNLOAD_DIR="/share/downloads"

# Install complete Traditional Chinese fonts, language packs, and native Firefox, then preserve original bwrap binary
RUN apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get install --no-install-recommends -y \
        firefox \
        firefox-locale-zh-hant \
        fonts-noto-cjk \
        fonts-noto-cjk-extra \
        fonts-wqy-zenhei \
        language-pack-zh-hant \
        language-pack-gnome-zh-hant && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/* && \
    mv /usr/bin/bwrap /usr/bin/bwrap.real

# Install WordCraft (.deb aarch64) with X11/Wayland/Mesa software GL and file-dialog deps, preserving original binary
RUN apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get install --no-install-recommends -y \
        libegl1 \
        libegl-mesa0 \
        libgl1 \
        libgl1-mesa-dri \
        libglx-mesa0 \
        libwayland-client0 \
        libwayland-egl1 \
        libx11-6 \
        libx11-xcb1 \
        libxcursor1 \
        libxi6 \
        libxkbcommon0 \
        libxkbcommon-x11-0 \
        libxrandr2 \
        xdg-desktop-portal \
        zenity && \
    curl -fsSL -o /tmp/wordcraft.deb \
        "https://github.com/storytold/wordcraft/releases/download/v${WORDCRAFT_VERSION}/wordcraft-${WORDCRAFT_VERSION}-linux-aarch64.deb" && \
    echo "${WORDCRAFT_DEB_SHA256}  /tmp/wordcraft.deb" | sha256sum -c - && \
    DEBIAN_FRONTEND=noninteractive apt-get install --no-install-recommends -y /tmp/wordcraft.deb && \
    rm -f /tmp/wordcraft.deb && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/* && \
    mv /usr/bin/wordcraft /usr/bin/wordcraft.real

# Copy Home Assistant options loader, fontconfig TC preference, bwrap/WordCraft wrappers, and Chromium wrapper
COPY rootfs /

# Guarantee executable permissions regardless of host filesystem and refresh font cache
RUN chmod a+x \
    /defaults/startwm.sh \
    /etc/s6-overlay/s6-rc.d/init-ha-options/run \
    /etc/s6-overlay/s6-rc.d/init-ha-downloads/run \
    /usr/bin/bwrap \
    /usr/bin/wordcraft \
    /usr/local/bin/wrapped-chromium && \
    fc-cache -fv

LABEL \
    org.opencontainers.image.title="Home Assistant App: Webtop (Ubuntu XFCE)" \
    org.opencontainers.image.description="Ubuntu XFCE desktop in your browser via Selkies (linuxserver/webtop)." \
    org.opencontainers.image.source="https://github.com/s90061/ha-apps" \
    org.opencontainers.image.licenses="GPL-3.0-only"
