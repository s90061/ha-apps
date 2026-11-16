# Changelog

## 0.1.0

- Initial Home Assistant App release wrapping `linuxserver/webtop:ubuntu-xfce` (`sha256:12b00cce8f60ee8f4553f2726d35c5d48d34d6c87513b1044a20619a0a2e3c3b`, `aarch64`).
- Added Home Assistant Ingress (port `3000`) plus optional direct HTTP (`3000`) and HTTPS (`3001`) ports.
- Mapped `app_config:rw` (`/config`) and `share:rw` (`/share`) for persistent desktop data and file exchange.
- Added `/data/options.json` integration (`tz`, `title`, `puid`, `pgid`, `custom_user`, `password`) with English and Traditional Chinese UI translations.
- Disabled DinD (`START_DOCKER=false`), auto-detected IPv6 availability for Nginx, and added `--disable-dev-shm-usage` to `wrapped-chromium`.
