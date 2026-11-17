# Changelog

## 0.1.3

- Preinstall complete Traditional Chinese fonts (`fonts-noto-cjk`, `fonts-noto-cjk-extra`, `fonts-wqy-zenhei`) and Ubuntu Traditional Chinese language packs (`language-pack-zh-hant`, `language-pack-gnome-zh-hant`) with a `Noto Sans CJK TC` fontconfig preference (`/etc/fonts/conf.d/64-language-selector-prefer.conf`).
- Add `lc_all` option to the Home Assistant configuration UI (defaulting to `zh_TW.UTF-8`) and ensure `init-ha-options` executes before `init-nginx` and `init-selkies-config` so `LC_ALL`, `LANG`, and `LANGUAGE` apply across the desktop session.

## 0.1.2

- Disable Selkies HiDPI (`SELKIES_USE_CSS_SCALING=true`) by default so the stream runs at 96 DPI / 1x logical resolution with CSS scaling instead of 2x physical pixels; all other settings remain unchanged.

## 0.1.1

- Fix `xfce4-session` startup crash (`Aborted (core dumped)`) caused by `libglycin-2` / `bwrap` sandbox failure under Home Assistant's default AppArmor profile by adding a `/usr/bin/bwrap` fallback wrapper so `libglycin` cleanly degrades to unsandboxed image decoding when blocked.
- Explicitly pin base tag `lscr.io/linuxserver/webtop:ubuntu-xfce-76362218-ls10@sha256:12b00cce8f60ee8f4553f2726d35c5d48d34d6c87513b1044a20619a0a2e3c3b`.
- Set `DISABLE_DRI3=true` by default so Xvfb does not fail probing `/dev/dri/renderD128` without device cgroup access.
- Retain `xfce4-session` startup logs in `/config/.local/state/xfce4-session.log` instead of discarding to `/dev/null`.

## 0.1.0

- Initial Home Assistant App release wrapping `linuxserver/webtop:ubuntu-xfce` (`sha256:12b00cce8f60ee8f4553f2726d35c5d48d34d6c87513b1044a20619a0a2e3c3b`, `aarch64`).
- Added Home Assistant Ingress (port `3000`) plus optional direct HTTP (`3000`) and HTTPS (`3001`) ports.
- Mapped `app_config:rw` (`/config`) and `share:rw` (`/share`) for persistent desktop data and file exchange.
- Added `/data/options.json` integration (`tz`, `title`, `puid`, `pgid`, `custom_user`, `password`) with English and Traditional Chinese UI translations.
- Disabled DinD (`START_DOCKER=false`), auto-detected IPv6 availability for Nginx, and added `--disable-dev-shm-usage` to `wrapped-chromium`.
