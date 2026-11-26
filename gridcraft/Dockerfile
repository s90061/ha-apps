FROM docker.io/library/rust:1.95-trixie AS builder

ARG GRIDCRAFT_VERSION="0.19.1"
ARG GRIDCRAFT_SRC_SHA256="4adb7b981de97f31c61e5be67ac7b2bcd84e2559647b5c8c39561bbcce199c5e"
ARG TRUNK_VERSION="0.22.2"
ARG TRUNK_X86_64_SHA256="4a90216d93c553f3d8b7401fce0dee588e0fb1eb1c6a379ebac81fabe820f25b"
ARG TRUNK_AARCH64_SHA256="3485963cc854763d098a9702b5c98834fa7bfed7240a457ef00a5612b403c9e1"
ARG NOTO_TC_COMMIT="f8d157532fbfaeda587e826d4cd5b21a49186f7c"
ARG NOTO_TC_SHA256="5bab0cb3c1cf89dde07c4a95a4054b195afbcfe784d69d75c340780712237537"

RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates curl git python3 && \
    rm -rf /var/lib/apt/lists/* && \
    rustup target add wasm32-unknown-unknown

RUN arch="$(uname -m)" && \
    case "${arch}" in \
        x86_64)  trunk_target="x86_64-unknown-linux-musl";  trunk_sha="${TRUNK_X86_64_SHA256}" ;; \
        aarch64) trunk_target="aarch64-unknown-linux-musl"; trunk_sha="${TRUNK_AARCH64_SHA256}" ;; \
        *) echo "Unsupported builder arch: ${arch}" >&2; exit 1 ;; \
    esac && \
    curl -fsSL -o /tmp/trunk.tar.gz \
        "https://github.com/trunk-rs/trunk/releases/download/v${TRUNK_VERSION}/trunk-${trunk_target}.tar.gz" && \
    echo "${trunk_sha}  /tmp/trunk.tar.gz" | sha256sum -c - && \
    tar -xzf /tmp/trunk.tar.gz -C /usr/local/bin trunk && \
    chmod a+x /usr/local/bin/trunk && \
    rm -f /tmp/trunk.tar.gz

RUN mkdir -p /build/fonts /build/dist && \
    curl -fsSL -o /build/fonts/NotoSansTC-Regular.otf \
        "https://raw.githubusercontent.com/notofonts/noto-cjk/${NOTO_TC_COMMIT}/Sans/SubsetOTF/TC/NotoSansTC-Regular.otf" && \
    echo "${NOTO_TC_SHA256}  /build/fonts/NotoSansTC-Regular.otf" | sha256sum -c - && \
    curl -fsSL -o /tmp/gridcraft-src.tar.gz \
        "https://github.com/storytold/gridcraft/archive/refs/tags/v${GRIDCRAFT_VERSION}.tar.gz" && \
    echo "${GRIDCRAFT_SRC_SHA256}  /tmp/gridcraft-src.tar.gz" | sha256sum -c - && \
    tar -xzf /tmp/gridcraft-src.tar.gz -C /build && \
    mv "/build/gridcraft-${GRIDCRAFT_VERSION}" /build/gridcraft && \
    rm -f /tmp/gridcraft-src.tar.gz

COPY patches /build/patches

RUN python3 /build/patches/apply-tw-patch.py /build/gridcraft /build/patches /build/fonts/NotoSansTC-Regular.otf && \
    cd /build/gridcraft/apps/gridcraft-web && \
    trunk build --release --dist /build/dist --public-url ./

FROM docker.io/library/alpine:3.21

ARG GRIDCRAFT_VERSION="0.19.1"

ENV GRIDCRAFT_VERSION="${GRIDCRAFT_VERSION}" \
    PYTHONUNBUFFERED="1"

RUN apk add --no-cache \
        bash \
        ca-certificates \
        curl \
        gzip \
        jq \
        python3 \
        tzdata && \
    mkdir -p /app/web /share/gridcraft /config

COPY --from=builder /build/dist/. /app/web/
COPY rootfs /

RUN cp /app/ha-bridge.css /app/ha-bridge.js /app/web/ && \
    python3 -c 'import pathlib; p = pathlib.Path("/app/web/index.html"); s = p.read_text("utf-8"); assert "<canvas id=\"gridcraft_canvas\"></canvas>" in s; s = s.replace("<canvas id=\"gridcraft_canvas\"></canvas>", "<canvas id=\"gridcraft_canvas\" data-host=\"page\"></canvas>\n  <script src=\"./api/config.js\"></script>\n  <script src=\"./ha-bridge.js\"></script>"); s = s.replace("</head>", "  <link rel=\"stylesheet\" href=\"./ha-bridge.css\" />\n</head>"); p.write_text(s, "utf-8")' && \
    gzip -9 -k /app/web/*.wasm /app/web/*.js /app/web/snippets/*/*.js && \
    chmod a+x /usr/local/bin/ha-entrypoint.sh

WORKDIR /app

EXPOSE 8090/tcp

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD curl -fsS "http://127.0.0.1:8090/api/health" >/dev/null || exit 1

ENTRYPOINT ["/usr/local/bin/ha-entrypoint.sh"]

LABEL \
    org.opencontainers.image.title="Home Assistant App: GridCraft" \
    org.opencontainers.image.description="Open-source pure-Rust spreadsheet (.xlsx/.csv/.tsv/.xls/.xlsb/.ods) via WebAssembly with Taiwan Traditional Chinese Excel localization and Home Assistant server storage." \
    org.opencontainers.image.source="https://github.com/s90061/ha-apps" \
    org.opencontainers.image.licenses="MIT OR Apache-2.0"
