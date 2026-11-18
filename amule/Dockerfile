# Pinned strictly to ngosang/amule 3.2.0-1 multi-arch OCI index (arm64 child: sha256:ca1c3f947a8ae02ea9576b2cdfa5e14ba2834c57e6252ef6d8d5db22b80d480b)
FROM docker.io/ngosang/amule:3.2.0-1@sha256:4b1a4246d57e7994b31c723b8c55474654e328c1b8eb0f9deb7f1843673bd515

ARG BUILD_VERSION="dev"
ENV APP_VERSION="${BUILD_VERSION}"

# Install jq for /data/options.json parsing and nginx-light for HA Ingress cookie-path proxying
RUN apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
        jq \
        nginx-light && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/* /var/log/nginx/*

# Copy Home Assistant entrypoint, Ingress nginx config, and s6 service
COPY rootfs /

# Guarantee executable permissions regardless of host filesystem, and use chown -RH so
# upstream fix_permissions dereferences the /home/amule/.aMule -> /config symlink.
RUN chmod a+x \
    /usr/local/bin/ha-entrypoint.sh \
    /etc/services.d/ingress-nginx/run \
    /etc/services.d/ingress-nginx/finish \
    /etc/services.d/ingress-nginx/log/run && \
    sed -i 's/chown -R /chown -RH /g' /home/amule/amule-config.sh && \
    grep -q 'chown -RH ' /home/amule/amule-config.sh

EXPOSE 8099/tcp 4711/tcp 4712/tcp 4662/tcp 4665/udp 4672/udp

HEALTHCHECK --interval=30s --timeout=10s --start-period=120s --retries=3 \
    CMD pgrep -x amuled >/dev/null && curl -fsS http://127.0.0.1:4711/api/v1/health >/dev/null || exit 1

ENTRYPOINT ["/usr/local/bin/ha-entrypoint.sh"]

LABEL \
    org.opencontainers.image.title="Home Assistant App: aMule" \
    org.opencontainers.image.description="eMule / ED2K and Kademlia P2P client with Web UI (ngosang/amule 3.2.0-1)." \
    org.opencontainers.image.source="https://github.com/s90061/ha-apps" \
    org.opencontainers.image.licenses="MIT AND GPL-2.0-or-later"
