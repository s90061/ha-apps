# Changelog

## 0.1.11

- Enabled `udev: true` and explicit DMA-BUF heap device permissions (`/dev/dma_heap/vidbuf_cached`, `/dev/dma_heap/linux,cma`, `/dev/dma_heap/reserved`, `/dev/dma_heap/system`, `/dev/udmabuf`) in `config.yaml` so Raspberry Pi 5's `rpi-hevc-dec` (`v4l2request` / `libudev` + zero-copy DMABufs) can actually initialize inside Home Assistant OS containers without disabling Protection Mode.
- Added concise startup hardware-device access probing in `ha-entrypoint.sh` and live filtered FFmpeg `Hwaccel`/`v4l2`/`dma_heap` diagnostic logging in `HLSManager` (`-loglevel info -nostats` when `HW_ACCEL=drm`) so Home Assistant App logs clearly confirm whether Pi 5 HEVC hardware decoding engaged.

## 0.1.10

- Removed the hardcoded Windows Firefox block from `detectHEVCHlsSupport()` so modern Windows Firefox (134+ with HEVC Video Extensions) can play 4K/1080p HEVC + EAC3/AC3 videos via zero-reencode `Remux` (`-c:v copy` + AAC audio, ~1–3% Pi 5 CPU) instead of falling back to full CPU video transcoding.
- Added browser native Dolby Digital Plus (`EAC3` / `ec-3`) detection (`detectEAC3Support()`) so browsers with native EAC3 support (e.g. Edge/Safari with Dolby) can play `HEVC + EAC3` `.mp4` files directly via `Direct Play` (0% Pi 5 CPU).

## 0.1.9

- Enabled native Raspberry Pi 5 4Kp60 HEVC (`rpi-hevc-dec` / V4L2-request DRM) hardware decoding for HEVC transcoding: switched runtime to Debian Trixie with Raspberry Pi official patched `ffmpeg` (`7.1.x+rpt`), added `video: true` and `hw_hevc_decode` option (default `true`), and added automatic runtime fallback to CPU software decoding if hardware decode fails on any stream.
- Split client HEVC capability detection into Direct Play (`hevc`) vs HLS MSE (`hevc_hls`) so Windows Firefox and other browsers supporting MP4 HEVC Direct Play always stay in zero-CPU `Direct Play` for 4K HEVC `.mp4` files, and allowed `hls.recoverMediaError()` retries before falling back from HEVC Remux.
- Added `Cache-Control: no-cache, no-store, must-revalidate` headers to SPA `index.html` responses so Home Assistant App updates load the newest frontend immediately without stale browser cache.

## 0.1.8

- Completely eliminated permanently stuck / ghost subtitles after shifting subtitle offset (`-0.5s` / `+0.5s` / reset) by caching raw WebVTT in memory, shifting timestamps in-memory (supporting both `HH:MM:SS.mmm` and `MM:SS.mmm`), disabling and removing the old `<track>`, and mounting a fresh Blob `<track>` instead of mutating live `VTTCue` timestamps in place.

## 0.1.7

- Fixed ghost / overlapping duplicate subtitles when adjusting subtitle time offset (`-0.5s` / `+0.5s` / reset) while the video is paused: snapshot `track.cues` with `Array.from()` to prevent live-list reordering from skipping cues, update `startTime`/`endTime` in direction-safe order, and refresh `track.mode` (`hidden` -> `showing` on next animation frame) to force browsers to clear stale cue overlays immediately.

## 0.1.6

- Added immediate transcode termination (`POST /api/stream/stop`) triggered when closing the video player, navigating away, refreshing, closing the browser tab (`navigator.sendBeacon` + `keepalive` fetch), or switching back to Direct Play, dropping Raspberry Pi 5 CPU usage to 0% immediately instead of waiting 60–85 seconds for idle session cleanup.
- Added a short-lived session stop tombstone in `HLSManager` so delayed in-flight `.ts` segment requests arriving milliseconds after closing a video cannot resurrect FFmpeg.
- Tightened idle session reaper interval (`10s` check / `45s` timeout) and sliding-window ahead-buffer high/low watermarks (`6` / `3` segments) to minimize background CPU work.

## 0.1.5

- Added zero-reencode HEVC (`-c:v copy`) MKV remux support for HEVC-capable browsers and smart 1080p bypass (`direct`/`remux`) for videos already at `<= 1080p` and `<= 8 Mbps`, dropping Raspberry Pi 5 CPU usage from >90% to ~1–2% when re-encoding is unnecessary.
- Optimized Raspberry Pi 5 software 1080p/720p transcoding with `-preset superfast`, tuned `x264-params` (`bframes=2:ref=1:rc-lookahead=0:weightp=0`), `-fpsmax 30`, conditional `-vf scale` omission when source width is already `<=` target width, and `fast_bilinear` downscaling for 4K sources.
- Added burst + paced input readrate throttling (`-readrate_initial_burst 12 -readrate 1.2`) and sliding-window `SIGSTOP`/`SIGCONT` process pause when FFmpeg buffers 8+ segments ahead of the playback head, preventing sustained >90% CPU spikes and `/dev/shm` overflow.
- Improved browser HEVC MSE detection and added automatic fallback to compatible H.264 1080p transcoding if HEVC remux decoding fails.

## 0.1.4

- Added a toggleable file-size filter control (`過濾`) to the left of the sort selector in the "All Videos" (`全部影片`) view to hide small video files below a customizable MB threshold (default `100 MB`, with quick presets `50 MB` / `100 MB` / `200 MB` / `500 MB` / `1 GB` and persistent preference), without affecting subtitle files.

## 0.1.3

- Limited the "Continue Watching" (`繼續觀看`) list to the 4 most recently watched unfinished videos so it stays on a single clean row on desktop.

## 0.1.2

- Redesigned the PiStream web interface into a matte graphite home-cinema console with a single warm amber accent (`#e2a336`), uniform Lucide line icons, `Inter` + `Noto Sans TC` + `JetBrains Mono` typography (`tabular-nums`), and clean 16:9 thumbnails with specs relocated to the card footer.
- Added file date sorting (`date_desc`, `date_asc`, `name_asc`, `name_desc`, `size_desc`) backed by filesystem `file_mod_time` (with automatic SQLite migration for existing databases), folder sorting by newest video date, and `YYYY/MM/DD` date indicators on video cards.
- Replaced native `<select>` controls across library sorting and the video player with a custom `CinemaSelect` popover component to eliminate Windows Chrome vertical-alignment bugs and OS-native arrow/menu glitches.
- Added automatic top-bar and cursor hiding after 2.5 seconds of inactivity during active video playback (working across OS fullscreen, Home Assistant Ingress, and mobile WebView) with instant mouse/touch/keyboard wake-up.

## 0.1.1

- Fixed subtitle-to-video timeline desynchronization in `remux` (`-c:v copy`) mode by using `-copyts -start_at_zero` to preserve true keyframe PTS timestamps instead of stamping preceding keyframes with a synthetic `output_ts_offset`.
- Locked `hls.js` initial PTS reference (`_initPTS = 0.000s`) on stream startup before performing resume seeks, preventing initial keyframe offset from shifting the MSE timeline.
- Fixed cumulative `hls.js` `alignSliding` segment-index drift on long MKV playback by resetting unbuffered fragment boundaries to the 4-second grid (`sn * 4`) and passing an explicit `seek` sequence token on out-of-buffer timeline seeks.
- Added live subtitle time-offset controls (`-0.5s` / `0.0s` reset / `+0.5s`) in the video player top bar for real-time WebVTT cue synchronization without reloading.

## 0.1.0

- Initial Home Assistant App release packaging [`s90061/pistream`](https://github.com/s90061/pistream) (`aarch64`).
- Added full Home Assistant Ingress subpath compatibility (`base: './'` and dynamic path resolution for REST API, thumbnails, WebVTT subtitles, Direct Play, and HLS Web Worker streams) alongside direct port `8080/tcp` zero-copy streaming.
- Mapped `app_config:rw` (`/config/pistream.db`, persistent `/config/.jwt_secret`, and backup-excluded `/config/cache`), `media:ro` (`/media`), and `share:ro` (`/share`, supporting direct playback from `/share/amule/incoming`).
- Added smart `transcode_storage: auto` fallback (`/tmp/pistream_hls` when container `/dev/shm` is under 256MB) to prevent 4K Remux OOM under Home Assistant's default 64MB `/dev/shm`.
- Added `/data/options.json` integration, first-boot random admin password generation + optional `admin_password` sync, and English / Traditional Chinese UI translations.
