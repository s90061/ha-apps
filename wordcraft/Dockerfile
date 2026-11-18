FROM docker.io/library/alpine:3.21

ARG WORDCRAFT_VERSION="0.18.1"
ARG WORDCRAFT_WEB_SHA256="3daa7c2d1f209a7eeb2527fcefa3fe31e875848fb1970d68f54edae55fe0bc9f"

ENV WORDCRAFT_VERSION="${WORDCRAFT_VERSION}" \
    PYTHONUNBUFFERED="1"

RUN apk add --no-cache \
        bash \
        ca-certificates \
        curl \
        gzip \
        jq \
        python3 \
        tzdata \
        unzip && \
    mkdir -p /app/web /share/wordcraft /config && \
    curl -fsSL -o /tmp/wordcraft-web.zip \
        "https://github.com/storytold/wordcraft/releases/download/v${WORDCRAFT_VERSION}/wordcraft-web-${WORDCRAFT_VERSION}.zip" && \
    echo "${WORDCRAFT_WEB_SHA256}  /tmp/wordcraft-web.zip" | sha256sum -c - && \
    unzip -q /tmp/wordcraft-web.zip -d /tmp/wc && \
    cp -R "/tmp/wc/wordcraft-web-${WORDCRAFT_VERSION}/." /app/web/ && \
    rm -rf /tmp/wordcraft-web.zip /tmp/wc

COPY rootfs /

RUN cp /app/ha-bridge.css /app/ha-bridge.js /app/web/ && \
    python3 -c 'import pathlib; p = pathlib.Path("/app/web/index.html"); s = p.read_text("utf-8"); assert "id=\"wordcraft_canvas\"" in s; s = s.replace("<canvas id=\"wordcraft_canvas\"></canvas>", "<canvas id=\"wordcraft_canvas\" data-host=\"page\"></canvas>\n  <script src=\"./ha-bridge.js\"></script>"); s = s.replace("</head>", "  <link rel=\"stylesheet\" href=\"./ha-bridge.css\" />\n</head>"); p.write_text(s, "utf-8")' && \
    gzip -9 -k /app/web/*.wasm /app/web/*.js && \
    chmod a+x /usr/local/bin/ha-entrypoint.sh /app/server.py

WORKDIR /app

EXPOSE 8088/tcp

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD curl -fsS "http://127.0.0.1:8088/api/health" >/dev/null || exit 1

ENTRYPOINT ["/usr/local/bin/ha-entrypoint.sh"]

LABEL \
    org.opencontainers.image.title="Home Assistant App: WordCraft" \
    org.opencontainers.image.description="Open-source pure-Rust word processor (.docx/ODT/RTF/PDF/Markdown) via WebAssembly with Home Assistant server storage." \
    org.opencontainers.image.source="https://github.com/s90061/ha-apps" \
    org.opencontainers.image.licenses="MIT OR Apache-2.0"
