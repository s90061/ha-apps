# Home Assistant App: PiStream

## Overview

This app packages [`s90061/pistream`](https://github.com/s90061/pistream) (`aarch64`) for Home Assistant Supervisor — a lightweight media server purpose-built for Raspberry Pi 5 with zero external metadata scraping, instant mid-stream seeking, external/embedded WebVTT subtitles, and multi-user playback progress.

## Three-Tier Streaming Engine

1. **Tier 1 — Direct Play (`direct`, HTTP 206 Byte-Range)**: Zero CPU usage and `<100ms` seeking for browser-compatible MP4 / WebM / MOV files (H.264 / VP9 / AV1, plus HEVC and native Dolby Digital Plus `EAC3` on hardware-capable browsers).
2. **Tier 2 — Zero-Copy Remux (`remux`, `-c:v copy -c:a aac`)**: Repackages MKV or HEVC + AC3/EAC3/DTS audio into HLS on the fly with Input Keyframe Seeking (`>50x` realtime, ~1–3% Pi 5 CPU) and Smart 1080p Bypass (`<= 1080p` and `<= 8 Mbps`).
3. **Tier 3 — Hardware-Decoded Adaptive Transcoding (`transcode`)**: Switch between **Original**, **1080p**, and **720p** using Raspberry Pi 5's built-in 4Kp60 8/10-bit HEVC hardware decoder (`rpi-hevc-dec` via V4L2-request DRM + BCM2712 IOMMU MMap, with automatic CPU fallback), burst + paced `1.2x` readrate throttling, `SIGSTOP`/`SIGCONT` ahead-buffer pausing, sliding-window segment cleanup (`max_keep_segments`), and instant FFmpeg termination when closing the player.

## Security Architecture

* **Pure `HttpOnly` Cookie Authentication**: No query `?token=` or `Bearer` token endpoints; internal HMAC-SHA256 signing key is automatically generated and persisted in `/config/.jwt_secret` (`chmod 600`).
* **Immediate Server-Side Session Revocation**: Deleting an account, changing/resetting passwords, or logging out immediately invalidates active sessions via SQLite `token_version`.
* **Defense in Depth**: Sliding-window login brute-force rate limiting (`429`), strict `sessionID` validation & per-user session ownership, HLS concurrency limits, HA Ingress-compatible CSP (`frame-ancestors 'self'`) and same-origin CSRF checks, single-flight authenticated library scanning, and admin-only user management.

## Accessing PiStream

1. **Home Assistant Ingress (Sidebar / Open Web UI)**: Works out of the box via internal port `8080`.
2. **Direct Port (`http://<your-ha-ip>:8080`)**: Mapped to host port `8080` by default for zero-copy Go `http.ServeContent` streaming without passing high-bitrate 4K video through the Home Assistant Python Ingress proxy.
3. **Initial Admin Login**:
   - Set `admin_password` (at least 4 characters) in the **Configuration** tab before starting, **or**
   - Leave `admin_password` blank on first start and copy `[ha-init] Initial PiStream Admin password: <password>` from the app **Log** tab.
   - While `admin_password` is filled in Configuration, it updates the admin password on every restart; clear `admin_password` if you prefer managing your password inside the PiStream Web UI (`👤 Account Settings`).

## Storage & Media Integration

* **`/media` (`media:ro`) & `/share` (`share:ro`)**: Mounted read-only to protect your video files. Set `media_dir` to `/media` (default, including external USB drives mounted under `/media/<disk>`) or `/share/amule/incoming` to stream completed downloads directly from the **aMule** app.
* **`/config` (`app_config:rw`)**: Persists `/config/pistream.db` (users, watch progress, media index), `/config/.jwt_secret`, and `/config/cache` (16:9 preview thumbnails and extracted WebVTT subtitles; excluded from HA backups via `backup_exclude`).
* **Smart HLS Buffer (`transcode_storage: auto`)**: Home Assistant gives containers a 64MB `/dev/shm` by default. In `auto` mode, PiStream checks `/dev/shm` capacity at boot and automatically uses `/tmp/pistream_hls` when `/dev/shm` is under 256MB so high-bitrate 4K Remux streams never run out of memory.

## Configuration Options

| Option | Type | Default | Description |
| --- | --- | --- | --- |
| `tz` | `str` | `Asia/Taipei` | Container timezone (`TZ`). |
| `media_dir` | `match(^/(media\|share\|config)(/.*)?$)` | `/media` | Media library root folder (`MEDIA_DIR`). Changing this path automatically clears all playback progress, resets the media index, and wipes thumbnail/subtitle caches before rescanning. |
| `admin_user` | `str` | `admin` | Default administrator username (`ADMIN_USER`). |
| `admin_password` | `password?` | _(unset)_ | Optional admin password override (`ADMIN_PASSWORD`). |
| `segment_duration` | `int(2,10)` | `4` | HLS segment duration in seconds (`SEGMENT_DURATION`). |
| `buffer_seconds` | `int(16,120)` | `45` | Target forward HLS playback buffer in seconds (`BUFFER_SECONDS`), synchronizing both browser `hls.js` pre-buffering and server FFmpeg ahead-segment watermarks. |
| `max_keep_segments` | `int(4,60)` | `10` | Sliding-window segments kept behind playhead (`MAX_KEEP_SEGMENTS`). |
| `transcode_storage` | `list(auto\|shm\|tmp)` | `auto` | HLS buffer location (`auto`, `shm` for `/dev/shm`, or `tmp` for `/tmp`). |
| `hw_hevc_decode` | `bool` | `true` | Enable Raspberry Pi 5 V4L2-request / DRM hardware HEVC decoding (`rpi-hevc-dec`, auto-falls back to CPU if unavailable). |
