# Home Assistant App: Webtop (Ubuntu XFCE)

## Overview

This app packages `linuxserver/webtop:ubuntu-xfce` (`sha256:12b00cce8f60ee8f4553f2726d35c5d48d34d6c87513b1044a20619a0a2e3c3b`, `aarch64`) for Home Assistant Supervisor, delivering a full Ubuntu XFCE desktop accessible directly in your browser via Selkies.

## Accessing the Desktop

1. **Home Assistant Ingress (Sidebar / Open Web UI)**: Enabled out of the box via internal port `3000`.
2. **Direct HTTPS (`https://<your-ha-ip>:3001`)**: Mapped to host port `3001` by default using a self-signed certificate (accept the browser warning on first visit). Direct HTTPS provides a secure browser context for WebCodecs hardware-accelerated streaming and audio.
3. **Optional Direct HTTP (`3000/tcp`)**: Unmapped by default to avoid port conflicts; assign a host port in the **Network** section if needed.

## Storage Persistence

* **`/config` (`app_config:rw`)**: Persisted in `/addon_configs/webtop_ubuntu_xfce` on the Home Assistant host, surviving updates and included in Home Assistant backups.
* **`/share` (`share:rw`)**: Shared with Home Assistant for easy file transfers; `/config/Downloads` and `/config/下載` (plus `XDG_DOWNLOAD_DIR`) automatically map to `/share/downloads` by default.

## Configuration Options

| Option | Type | Default | Description |
| --- | --- | --- | --- |
| `tz` | `str` | `Asia/Taipei` | Container timezone (e.g. `Asia/Taipei`, `UTC`). |
| `title` | `str` | `Ubuntu XFCE` | Browser tab and manifest title. |
| `lc_all` | `str` | `zh_TW.UTF-8` | Desktop language and locale (`LC_ALL`, `LANG`, `LANGUAGE`). |
| `download_dir` | `str` | `/share/downloads` | Target directory mapped from `/config/Downloads`, `/config/下載`, and `XDG_DOWNLOAD_DIR`. |
| `puid` | `int(0,65535)` | `1000` | UID mapped to desktop user `abc` for `/config` ownership. |
| `pgid` | `int(0,65535)` | `1000` | GID mapped to desktop user `abc` for `/config` ownership. |
| `custom_user` | `str?` | _(unset)_ | Optional HTTP Basic Auth username when `password` is set (defaults to `abc` if omitted). |
| `password` | `password?` | _(unset)_ | Optional HTTP Basic Auth password protecting the web UI (leave blank to rely solely on Home Assistant Ingress auth). |

## Home Assistant Compatibility Adjustments

* `START_DOCKER` defaults to `false` to prevent nested Docker daemon startup inside an unprivileged app container.
* `DISABLE_DRI3` defaults to `true` and `SELKIES_USE_CSS_SCALING` defaults to `true` (disabling HiDPI for smooth Raspberry Pi / Ingress streaming).
* A `/usr/bin/bwrap` compatibility wrapper falls back cleanly when Home Assistant's default AppArmor profile blocks bubblewrap sandboxing for `libglycin`, preventing XFCE session aborts.
* Traditional Chinese fonts (`fonts-noto-cjk`, `fonts-noto-cjk-extra`, `fonts-wqy-zenhei`), `language-pack-zh-hant`, `language-pack-gnome-zh-hant`, and a `Noto Sans CJK TC` fontconfig preference are baked into the image with `LC_ALL=zh_TW.UTF-8` as the default.
* Native Firefox (`.deb` with `firefox-locale-zh-hant`) is preinstalled alongside Chromium and configured to follow the container `LC_ALL` locale, save downloads via `XDG_DOWNLOAD_DIR` (`/share/downloads`), and suppress container user-namespace sandbox warnings.
* [WordCraft](https://github.com/storytold/wordcraft) (`0.18.1`, `aarch64.deb`, including `wordcraft-cli`) is preinstalled natively so it can open and save documents directly in `/share` as well as `/config`, wrapped at `/usr/bin/wordcraft` to fall back cleanly to Mesa software OpenGL (`LIBGL_ALWAYS_SOFTWARE=1`, `WGPU_BACKEND=gl`) when `DISABLE_DRI3=true` or no `/dev/dri/renderD128` device is accessible.
* IPv6 support is auto-detected at boot; if unavailable inside the container network, `DISABLE_IPV6=true` is set automatically so Nginx starts cleanly.
* `wrapped-chromium` appends `--disable-dev-shm-usage` to prevent tab crashes under tight `/dev/shm` constraints.
