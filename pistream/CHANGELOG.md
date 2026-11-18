# Changelog

## 0.1.0

- Initial Home Assistant App release packaging [`s90061/pistream`](https://github.com/s90061/pistream) (`aarch64`).
- Added full Home Assistant Ingress subpath compatibility (`base: './'` and dynamic path resolution for REST API, thumbnails, WebVTT subtitles, Direct Play, and HLS Web Worker streams) alongside direct port `8080/tcp` zero-copy streaming.
- Mapped `app_config:rw` (`/config/pistream.db`, persistent `/config/.jwt_secret`, and backup-excluded `/config/cache`), `media:ro` (`/media`), and `share:ro` (`/share`, supporting direct playback from `/share/amule/incoming`).
- Added smart `transcode_storage: auto` fallback (`/tmp/pistream_hls` when container `/dev/shm` is under 256MB) to prevent 4K Remux OOM under Home Assistant's default 64MB `/dev/shm`.
- Added `/data/options.json` integration, first-boot random admin password generation + optional `admin_password` sync, and English / Traditional Chinese UI translations.
