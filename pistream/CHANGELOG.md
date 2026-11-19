# Changelog

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
