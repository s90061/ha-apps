FROM docker.io/library/alpine:3.21

ARG GRIDCRAFT_VERSION="0.19.1"
ARG GRIDCRAFT_WEB_SHA256="48c5d1b101befebb3c5ba3b7a14914f01c3207934813ed07dc4251b7ae116a78"

ENV GRIDCRAFT_VERSION="${GRIDCRAFT_VERSION}" \
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
    mkdir -p /app/web /share/gridcraft /config && \
    curl -fsSL -o /tmp/gridcraft-web.zip \
        "https://github.com/storytold/gridcraft/releases/download/v${GRIDCRAFT_VERSION}/gridcraft-web-${GRIDCRAFT_VERSION}.zip" && \
    echo "${GRIDCRAFT_WEB_SHA256}  /tmp/gridcraft-web.zip" | sha256sum -c - && \
    unzip -q /tmp/gridcraft-web.zip -d /tmp/gc && \
    cp -R "/tmp/gc/gridcraft-web-${GRIDCRAFT_VERSION}/." /app/web/ && \
    rm -rf /tmp/gridcraft-web.zip /tmp/gc

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
    org.opencontainers.image.description="Open-source pure-Rust spreadsheet (.xlsx/.csv/.tsv/.xls/.xlsb/.ods) via WebAssembly with Home Assistant server storage." \
    org.opencontainers.image.source="https://github.com/s90061/ha-apps" \
    org.opencontainers.image.licenses="MIT OR Apache-2.0"
