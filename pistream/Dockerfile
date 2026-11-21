# ==========================================
# Stage 1: Build React + TypeScript Frontend
# ==========================================
FROM docker.io/library/node:20-alpine AS web-builder

WORKDIR /src/web

COPY web/package*.json ./
RUN npm ci

COPY web/ ./
RUN npm run build

# ==========================================
# Stage 2: Build Pure Go Backend (Zero CGO, Native ARM64)
# ==========================================
FROM docker.io/library/golang:1.22-bookworm AS go-builder

ARG TARGETOS=linux
ARG TARGETARCH=arm64

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY cmd/ ./cmd/
COPY internal/ ./internal/

RUN go test ./... && \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /out/pistream ./cmd/server

# ==========================================
# Stage 3: Minimal Runtime (Debian Trixie + Raspberry Pi Patched FFmpeg w/ V4L2-request HEVC HW Decode)
# ==========================================
FROM docker.io/library/debian:trixie-slim

ARG BUILD_VERSION="dev"
ENV APP_VERSION="${BUILD_VERSION}" \
    DEBIAN_FRONTEND=noninteractive

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        ca-certificates \
        curl \
        gnupg \
        jq \
        tzdata && \
    curl -fsSL https://archive.raspberrypi.com/debian/raspberrypi.gpg.key \
        | gpg --dearmor -o /usr/share/keyrings/raspberrypi-archive-keyring.gpg && \
    printf 'deb [signed-by=/usr/share/keyrings/raspberrypi-archive-keyring.gpg] https://archive.raspberrypi.com/debian/ trixie main\n' \
        > /etc/apt/sources.list.d/raspi.list && \
    printf 'Package: *\nPin: origin archive.raspberrypi.com\nPin-Priority: 700\n' \
        > /etc/apt/preferences.d/raspi-ffmpeg && \
    apt-get update && \
    apt-get install -y --no-install-recommends ffmpeg && \
    ffmpeg -version && \
    ffmpeg -hwaccels && \
    apt-get purge -y --auto-remove gnupg && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=go-builder /out/pistream /app/pistream
COPY --from=web-builder /src/web/dist /app/web/dist
COPY rootfs /

RUN mkdir -p /config/cache/thumbs /config/cache/subs /media /share && \
    chmod a+x /app/pistream /usr/local/bin/ha-entrypoint.sh

EXPOSE 8080/tcp

HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
    CMD curl -fsS "http://127.0.0.1:8080/api/health" >/dev/null || exit 1

ENTRYPOINT ["/usr/local/bin/ha-entrypoint.sh"]

LABEL \
    org.opencontainers.image.title="Home Assistant App: PiStream" \
    org.opencontainers.image.description="Lightweight Raspberry Pi 5 media server with Direct Play, zero-copy Remux, and sliding-window HLS transcoding." \
    org.opencontainers.image.source="https://github.com/s90061/ha-apps" \
    org.opencontainers.image.licenses="MIT"
