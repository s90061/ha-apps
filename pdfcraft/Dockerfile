FROM docker.io/library/alpine:3.21

ARG PDFCRAFT_VERSION="0.21.1"
ARG PDFCRAFT_WEB_SHA256="fe9708d46af191aebb56f70eb32cf8b112b0b9452d17ae6ba3a8c4a8665fe748"

ENV PDFCRAFT_VERSION="${PDFCRAFT_VERSION}" \
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
    mkdir -p /app/web /share/pdfcraft /config && \
    curl -fsSL -o /tmp/pdfcraft-web.zip \
        "https://github.com/storytold/pdfcraft/releases/download/v${PDFCRAFT_VERSION}/pdfcraft-web-${PDFCRAFT_VERSION}.zip" && \
    echo "${PDFCRAFT_WEB_SHA256}  /tmp/pdfcraft-web.zip" | sha256sum -c - && \
    unzip -q /tmp/pdfcraft-web.zip -d /tmp/pc && \
    cp -R "/tmp/pc/pdfcraft-web-${PDFCRAFT_VERSION}/." /app/web/ && \
    rm -rf /tmp/pdfcraft-web.zip /tmp/pc

COPY rootfs /

RUN cp /app/ha-bridge.css /app/ha-bridge.js /app/web/ && \
    python3 -c 'import pathlib; p = pathlib.Path("/app/web/index.html"); s = p.read_text("utf-8"); assert "id=\"pdfcraft\"" in s; s = s.replace("<canvas id=\"pdfcraft\"></canvas>", "<canvas id=\"pdfcraft\" data-host=\"page\"></canvas>\n    <script src=\"./ha-bridge.js\"></script>"); s = s.replace("</head>", "    <link rel=\"stylesheet\" href=\"./ha-bridge.css\" />\n  </head>"); p.write_text(s, "utf-8")' && \
    gzip -9 -k /app/web/*.wasm /app/web/*.js /app/web/models/*.onnx && \
    chmod a+x /usr/local/bin/ha-entrypoint.sh

WORKDIR /app

EXPOSE 8089/tcp

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD curl -fsS "http://127.0.0.1:8089/api/health" >/dev/null || exit 1

ENTRYPOINT ["/usr/local/bin/ha-entrypoint.sh"]

LABEL \
    org.opencontainers.image.title="Home Assistant App: PdfCraft" \
    org.opencontainers.image.description="Open-source pure-Rust PDF reader, editor, OCR, and form/signature tool via WebAssembly with Home Assistant server storage." \
    org.opencontainers.image.source="https://github.com/s90061/ha-apps" \
    org.opencontainers.image.licenses="MIT OR Apache-2.0"
