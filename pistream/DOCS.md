# Home Assistant App: PiStream

## Overview

This app packages [`s90061/pistream`](https://github.com/s90061/pistream) (`aarch64`) for Home Assistant Supervisor — a lightweight media server purpose-built for Raspberry Pi 5 with zero external metadata scraping, instant mid-stream seeking, external/embedded WebVTT subtitles, and multi-user playback progress.

## Three-Tier Streaming Engine

1. **Tier 1 — Direct Play (`direct`, HTTP 206 Byte-Range)**: Zero CPU usage and `<100ms` seeking for browser-compatible MP4 / WebM / MOV files (H.264 / VP9 / AV1, plus HEVC on hardware-capable browsers).
2. **Tier 2 — Zero-Copy Remux (`remux`, `-c:v copy -c:a aac`)**: Repackages MKV H.264 or multi-channel AC3/DTS audio into HLS on the fly with Input Keyframe Seeking (`>50x` realtime on Raspberry Pi 5).
3. **Tier 3 — Bandwidth-Adaptive Transcoding (`transcode`)**: Switch between **Original**, **1080p**, and **720p** with sliding-window segment cleanup (`max_keep_segments`).

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
| `media_dir` | `match(^/(media\|share\|config)(/.*)?$)` | `/media` | Media library root folder (`MEDIA_DIR`). |
| `admin_user` | `str` | `admin` | Default administrator username (`ADMIN_USER`). |
| `admin_password` | `password?` | _(unset)_ | Optional admin password override (`ADMIN_PASSWORD`). |
| `segment_duration` | `int(2,10)` | `4` | HLS segment duration in seconds (`SEGMENT_DURATION`). |
| `max_keep_segments` | `int(4,60)` | `10` | Sliding-window segments kept behind playhead (`MAX_KEEP_SEGMENTS`). |
| `transcode_storage` | `list(auto\|shm\|tmp)` | `auto` | HLS buffer location (`auto`, `shm` for `/dev/shm`, or `tmp` for `/tmp`). |
