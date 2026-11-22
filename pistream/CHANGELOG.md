# Changelog

## 1.0.9

- **Auto-Reset Watch History & Media Index When `media_dir` Changes**:
  - Added an `app_meta` table and `Store.SyncMediaDir()` (`internal/db/sqlite.go`) to track the normalized active `media_dir` path across restarts. On first upgrade from `1.0.8` or earlier, the current path is recorded without touching existing data; whenever `media_dir` is subsequently changed (including parent/subfolder switches such as `/media` ↔ `/media/movies`), PiStream synchronously clears `playback_progress`, clears old `media_files` entries, and wipes stale `cache/thumbs` and `cache/subs` before starting the background library scan (preserving all user accounts).
  - Registered a `modernc.org/sqlite` connection hook (`PRAGMA foreign_keys = ON; PRAGMA busy_timeout = 5000;`) so every pooled SQLite connection enforces foreign-key `ON DELETE CASCADE` reliably.

## 1.0.8

- **Fixed Orphaned 100%-CPU Zombie FFmpeg Process on Quality Switch or Player Exit**:
  - Marked terminated `HLSSession` instances with `Terminated = true`, stored `ProcessCtx`, and cleared `ProcessDone` / `Cmd` inside `terminateSessionLocked()`, and guarded `EnsureSegmentReady()` and `restartFFmpegLocked()` so an in-flight segment request waking up after `StopSession()` (or eviction / idle cleanup) can never misclassify the intentional kill as a Raspberry Pi `drm` hardware-acceleration startup failure and respawn an unmonitored, unkillable CPU software-transcode (`HwAccel=none`) FFmpeg process outside `m.sessions`.
  - Added `StopSessionIfConfigMismatch()` in `GET /api/stream/{id}/info` so switching directly between HLS modes (e.g. `1080p 兼容轉碼` → `原畫 Remux` or switching audio tracks) immediately stops the previous FFmpeg process, and added `StopAllSessions()` during graceful server shutdown.
  - Updated `VideoPlayer.tsx` to destroy the previous `hls.js` instance immediately before requesting new stream info on quality/audio/mode changes, aborting in-flight segment downloads prior to server session teardown.

## 1.0.7

- **Fixed `HLSManager` Temp Directory Race Condition (`exit status 254: No such file or directory`)**:
  - Assigned a unique monotonic sequence suffix (`nextDirID`) to each `HLSSession` output directory in `getOrCreateSession()` so an asynchronous `StopSession(tombstone=false)` cleanup (e.g. switching from `HLS` → `Direct Play` → rapid fallback to `Remux`/`1080p` transcode under the same `sessionID`) can never delete the newly recreated session's output directory.
  - Ensured `os.MkdirAll(sess.OutputDir, 0755)` is always executed inside `restartFFmpegLocked()` before launching FFmpeg, preventing `ENOENT` failures from falsely disabling Pi 5 `drm` hardware HEVC decode (`DisableHwAccel = true`) and causing CPU-pegging retry loops.
  - Increased abnormal FFmpeg stderr diagnostic tail retention from `400` to `700` characters so root-cause lines (`Failed to open segment ...`) are preserved in logs.

## 1.0.6

- **Three-Stage Automatic Playback Fallback (`Direct Play` → `Remux` → `1080p` H.264 Transcode)**:
  - Added `<video>` `error` (`MEDIA_ERR_DECODE` / `MEDIA_ERR_SRC_NOT_SUPPORTED`) and playback-stall watchdog handling for `Direct Play` in `VideoPlayer.tsx` so MP4 files rejected by strict native browser demuxers (e.g. Firefox `MEDIA_ERR_DECODE` on non-standard HEVC MP4 containers) never spin indefinitely on "定位畫面中".
  - Added `force_remux` and `force_transcode` support to `GET /api/stream/{id}/info` and `DecideStreamModeWithOptions()` (`internal/stream/decision.go`): when `Direct Play` fails, PiStream automatically retries original-quality zero-reencode `Remux` (`-c:v copy`, ~1–3% Pi 5 CPU) first; if `Remux` also hits fatal `MEDIA_ERROR`, it automatically falls back to `1080p` H.264 transcoding (bypassing `Smart 1080p Bypass`).
  - Added an explicit **`畫質：1080p 兼容轉碼`** option to the player quality menu so users can manually force 1080p H.264 transcoding at any time without having to drop down to 720p.

## 1.0.5

- **Jellyfin-Style Custom HTML Subtitle Overlay (with Automatic PiP / iOS Native `<track>` Fallback)**:
  - Replaced native `video::cue` rendering on desktop browsers with a dedicated React HTML `<div>` subtitle overlay (`SubtitleOverlay.tsx`), keeping `<track mode="hidden">` as the WebVTT parser and timeline engine so subtitle styling, sizing, multi-line `box-decoration-break: clone` dark pills, and smooth GPU-composited `translateY` control-bar avoidance are 100% consistent across Firefox, Chrome, Edge, and Safari without mutating `VTTCue.line` or toggling `track.mode`.
  - Safely parses WebVTT inline formatting (`<i>`, `<b>`, `<u>`, `<ruby>`, `<rt>`, `<br>`) via `cue.getCueAsHTML()` AST traversal (zero `dangerouslySetInnerHTML`), preserves top-pinned cues (auto-shifted below the top bar when controls are visible), and syncs immediately on `cuechange`, `load`, `seeked`, `timeupdate`, and `-0.5s`/`+0.5s` offset changes.
  - Automatically falls back to native `<track mode="showing">` when entering Picture-in-Picture (`enterpictureinpicture`) or iOS native fullscreen (`webkitbeginfullscreen`), and restores the HTML overlay on exit.

## 1.0.4

- **Larger, Configurable Forward HLS Buffer (`buffer_seconds`, default `45`s, range `16`–`120`s)**:
  - Added `buffer_seconds` to Home Assistant app configuration (`config.yaml`, `translations/`, `DOCS.md`, `ha-entrypoint.sh`, and `internal/config/config.go`), exposed via `GET /api/stream/{id}/info`.
  - Synchronized browser `hls.js` buffering (`maxBufferLength: buffer_seconds`, `maxMaxBufferLength: max(60, buffer_seconds * 2)`, tighter `backBufferLength: 30` to preserve MSE quota, `fragLoadingMaxRetry: 2`) with dynamic server-side `HLSManager` ahead-segment `SIGSTOP`/`SIGCONT` watermarks (`45s` / `4s` -> low `6` / high `12` segments, capped at `16` ahead) and scaled `transcode_storage: auto` `/dev/shm` headroom checks.
- **Prevented Buffer-Underrun / Reload A/V & Subtitle Desync**:
  - Isolated explicit user seeks (custom timeline bar, `±10s`, keyboard shortcuts, resume seek) from `hls.js` `GapController` stall nudges (`_tryNudgeBuffer` / `_trySkipBufferHole`), which synchronously modify `video.currentTime` before emitting `BUFFER_NUDGE_ON_STALL` — stopping temporary buffer stalls from falsely incrementing `seekSeq` and restarting a healthy FFmpeg process.
  - Reset fragment timeline grid (`sn * segDur`), incremented `seekSeq`, locked `initPTS=0`, and reset `hls.js` Transmuxer (`resetTransmuxer()` / `fragPrevious = null` / `fragmentTracker`) before calling `hls.recoverMediaError()` on fatal `MEDIA_ERROR`, and resumed playback cleanly afterward so recovery reloads never drift out of sync with audio or subtitles.

## 1.0.3

- Automatically lift subtitles above the custom bottom timeline/control bar whenever playback controls are visible, and smoothly return them to the bottom edge when controls auto-hide:
  - Uses bottom-anchored `VTTCue` layout (`snapToLines = false`, `lineAlign = 'end'`, dynamically calculated from live `<video>` and seek-bar geometry + immediate Firefox reflow) on Firefox & Safari while preserving top-pinned cues.
  - Uses smooth hardware-accelerated `::-webkit-media-text-track-container` CSS translation (`--pistream-cue-lift`) on Chrome & Edge.

## 1.0.2

- **Fixed Remux Timeline Seek A/V Desync & Screen Jump**:
  - In `web/src/components/VideoPlayer.tsx`, reset `hls.js`'s `streamController` transmuxer (`resetTransmuxer()` and `fragPrevious = null`, preserving `initPTS` and pre-seeding `initPTS=0` on early seeks) on every out-of-buffer timeline seek so `MP4Remuxer.remuxVideo()` never misclassifies a post-restart adjacent segment (`snDiff === 1`) as contiguous and permanently shifts video frames one GOP behind audio & subtitles.
  - Guarded `onVideoSeeking` against internal buffer-gap / stall nudges (`FRAG_BUFFERED`, `BUFFER_STALLED_ERROR`, `BUFFER_NUDGE_ON_STALL`) and replaced hardcoded `4s` grid resets with `details.targetduration || 4`.
  - In `internal/stream/hls_manager.go`, tracked `StartSeg` on `HLSSession` so adjacent segment requests immediately following a seek (e.g. `Seg=820 -> Seg=821`) reuse the already-running contiguous FFmpeg process instead of restarting FFmpeg and backtracking to the same keyframe (eliminating the screen jump), with unit tests in `stream_test.go`.
- **Custom Bottom Player Controls (Removed Duplicate Bottom-Right CC & Fullscreen Icons)**:
  - Replaced native `<video controls>` with a PiStream Cinema-styled custom bottom control bar containing Play/Pause, drag-scrub timeline with buffered indicator and hover timestamp preview (committing seek once on release), true media duration display, and persistent Mute/Volume controls — removing the duplicate bottom-right `CC` and fullscreen icons across Firefox, Chrome, Edge, and Safari while keeping all subtitle & fullscreen controls in the top bar (plus click-to-play/pause, double-click fullscreen, touch control toggle, and iOS fullscreen fallback).

## 1.0.1

- Fixed intermittent CJK subtitle glyph corruption / garbled characters at large subtitle scales (e.g. `180%`) on Windows Firefox WebRender by switching `video::cue` in `web/src/index.css` from Google Fonts' 124-slice remote `Noto Sans TC` WOFF2 webfont to a single local native OS CJK system font stack (`"Microsoft JhengHei"`, `"PingFang TC"`, `"Microsoft YaHei"`, `"PingFang SC"`, `"Noto Sans CJK TC"`, `system-ui, sans-serif`), setting explicit regular `font-weight: 400`, and resetting inherited `font-feature-settings: normal`.

## 1.0.0

First stable production release of PiStream for Raspberry Pi 5 / Home Assistant OS (`aarch64`), following complete end-to-end functional, streaming, subtitle, security, and responsive layout verification:

- **Three-Tier Media Streaming & Raspberry Pi 5 Hardware Decoding**: HTTP 206 zero-CPU `Direct Play` (including HEVC & native `EAC3` detection), zero-reencode `Remux` (`-c:v copy` with accurate keyframe PTS alignment and Smart 1080p Bypass), and hardware-decoded `HLS Transcode` powered by Debian Trixie + Raspberry Pi patched `ffmpeg` (`rpi-hevc-dec` V4L2-request DRM via BCM2712 IOMMU MMap without disabling Protection Mode), with paced `1.2x` readrate, `SIGSTOP`/`SIGCONT` ahead-buffer throttling, and instant player-close termination.
- **Zero-Ghosting Subtitles & Cinema UI**: External (`.srt`/`.vtt`/`.ass`) and cached embedded WebVTT subtitles with in-memory timestamp offset shifting (`-0.5s`/`+0.5s`/reset) via Blob `<track>` rebuild, persistent font scaling (75%–220%), multi-user accounts and independent Continue Watching (max 4), folder browser, 5 symmetric sort options, customizable file-size filter, and responsive desktop/tablet/mobile layout.
- **Production Security Hardening**: Pure `HttpOnly` Cookie authentication with auto-managed `/config/.jwt_secret`, immediate server-side `token_version` session revocation, login brute-force rate limiting (`429`), strict `sessionID` validation and per-user session isolation, HLS concurrency caps, HA Ingress-compatible CSP & same-origin CSRF protection, and SHA256-pinned APT keyring verification.
- Updated `DOCS.md` to document `hw_hevc_decode`, Raspberry Pi 5 hardware decoding, and the 1.0.0 security architecture.

## 0.1.15

- Fixed `name_asc` (`檔名：A–Z`) sorting in `internal/db/sqlite.go` to sort purely by filename (`f.title COLLATE NOCASE ASC, f.id ASC`) instead of grouping by folder first, matching `name_desc` (`檔名：Z–A`) in the "All Videos" view.
- Fixed horizontal navbar overflow on tablet (`768px`) and mobile (`375px`) viewports in `Navbar.tsx` by wrapping the search bar onto a full-width second row below `lg` (`< 1024px`) and tightening mobile button spacing/username truncation.

## 0.1.14

- Removed the `jwt_secret` option from the Home Assistant Configuration UI (`config.yaml`, translations, and `DOCS.md`); the internal `HttpOnly` cookie signing key is now always automatically generated and persisted in `/config/.jwt_secret` (`chmod 600`).

## 0.1.13

- **Security Hardening**:
  - Fixed path traversal and cross-user session interference in `HLSManager`: strictly allowlist `sessionID` (`^[A-Za-z0-9_-]{1,64}$`), scope sessions by owning user ID, and enforce subpath containment before `MkdirAll`/`RemoveAll`.
  - Added immediate server-side session revocation (`token_version` in SQLite): deleting a user, resetting/changing passwords, or logging out immediately invalidates previous session cookies instead of remaining valid for 30 days, and `requireAuth`/`requireAdmin` always verify live DB state.
  - Switched strictly to `HttpOnly` Cookie authentication: removed `?token=` query auth, `Authorization: Bearer` auth, and raw `token` exposure in the login JSON response; dynamically set `Secure` on cookies for HTTPS / reverse-proxy requests.
  - Added sliding-window login brute-force rate limiting (`429 Too Many Requests`), 128-byte password cap, 64 KB JSON body limit (`http.MaxBytesReader`), and removed the hardcoded `admin123` fallback in `ha-entrypoint.sh`.
  - Added HLS concurrency limits (max 2 per user with oldest-session auto-eviction, max 4 global), strict M3U8 parameter allowlisting + `url.QueryEscape`, strict segment index bounds checking, admin-only deduplicated library scanning, `http.Server` `ReadHeaderTimeout`/`IdleTimeout`, and a 10 MB external subtitle read cap.
  - Added global security headers (`X-Content-Type-Options`, `Referrer-Policy`, `Permissions-Policy`, HA Ingress-compatible `X-Frame-Options: SAMEORIGIN` & `Content-Security-Policy` with `frame-ancestors 'self'`), same-origin CSRF checks on mutating requests, removed unnecessary `Access-Control-Allow-Origin: *` on subtitles, hid internal `file_path` and raw SQLite errors from API responses, and pinned SHA256 verification for `raspberrypi-archive-keyring` in `Dockerfile`.

## 0.1.12

- Fixed Raspberry Pi 5 4K 10-bit HEVC hardware decode hitting `Cannot allocate memory` (ENOMEM) on Home Assistant OS's 64 MB `linux,cma` pool: removed `/dev/dma_heap/linux,cma` and `/dev/dma_heap/reserved` from `config.yaml` so `rpi-ffmpeg` avoids falling through to the small CMA heap when `/dev/dma_heap/vidbuf_cached` is absent on HAOS and instead uses `MMap` + `VIDIOC_EXPBUF` via `rpi-hevc-dec`'s BCM2712 IOMMU (full system RAM).
- Limited DRM HEVC input decoder threads to `-threads 2` (before `-i`) to reduce DPB frame-buffer memory footprint and avoid contending with `libx264` encoding threads.

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
