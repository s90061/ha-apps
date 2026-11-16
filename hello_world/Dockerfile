ARG BUILD_FROM=ghcr.io/home-assistant/base:3.23
FROM ${BUILD_FROM}

ARG BUILD_VERSION="dev"
ENV APP_VERSION="${BUILD_VERSION}"

# Copy root filesystem
COPY rootfs /

# Guarantee executable permissions regardless of host filesystem
RUN chmod a+x \
    /etc/s6-overlay/s6-rc.d/hello_world/run \
    /etc/s6-overlay/s6-rc.d/hello_world/finish

LABEL \
    org.opencontainers.image.title="Home Assistant App: Hello World" \
    org.opencontainers.image.description="Minimal Home Assistant App verifying LF scripts, s6-overlay v3, and automatic updates." \
    org.opencontainers.image.source="https://github.com/s90061/ha-apps" \
    org.opencontainers.image.licenses="MIT"
