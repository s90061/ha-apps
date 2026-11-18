# Changelog

## 0.1.0

- Initial Home Assistant App release wrapping `ngosang/amule:3.2.0-1` (pinned multi-arch OCI digest `sha256:4b1a4246d57e7994b31c723b8c55474654e328c1b8eb0f9deb7f1843673bd515`, `aarch64` build).
- Added Home Assistant Ingress support via an internal `nginx-light` reverse proxy (`8099/tcp`) that dynamically rewrites `amuleapi` `Set-Cookie` paths (`X-Ingress-Path`) and keeps SSE (`/api/v1/events`) unbuffered.
- Mapped `app_config:rw` (`/config` linked to `/home/amule/.aMule`), `share:rw`, and `media:rw`, defaulting downloads to `/share/amule/incoming` and `/share/amule/temp`.
- Enabled ED2K/Kad P2P ports (`4662/tcp`, `4665/udp`, `4672/udp`) by default while keeping direct Web UI (`4711/tcp`) and External Connections (`4712/tcp`) ports disabled (`null`) unless explicitly mapped.
- Added `/data/options.json` integration, path synchronization on option updates, Docker `HEALTHCHECK`, and English / Traditional Chinese translations.
